package transparencyprocessor

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mindtastic/opentelemtry-transparency-processor/internal/filterspan"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.uber.org/zap"
	"net/http"
	"path"
	"sync"
	"time"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

const (
	attrCategories        = "tilt.categories"
	attrLegalBases        = "tilt.legalBases"
	attrLegimateInterests = "tilt.legitameteInterests"
	attrStorages          = "tilt.storageDurations"
)

type attributes struct {
	lastUpdated        time.Time
	categories         []string
	legalBases         []string
	legitametInterests []bool
	storages           []string
}

type transparencyProcessor struct {
	logger    *zap.Logger
	exportCtx context.Context

	timeout time.Duration

	telemetryLevel configtelemetry.Level

	mu              sync.RWMutex
	attributesCache map[string]attributes
	include         filterspan.Matcher
	exclude         filterspan.Matcher
	//attrProc        *attraction.AttrProc
}

func newTransparencyProcessor(set component.ProcessorCreateSettings, include, exclude filterspan.Matcher) *transparencyProcessor {
	tp := new(transparencyProcessor)
	tp.logger = set.Logger
	tp.attributesCache = make(map[string]attributes)

	tp.include = include
	tp.exclude = exclude

	return tp
}

func (a *transparencyProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		resource := rs.Resource()
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			ils := ilss.At(j)
			spans := ils.Spans()
			library := ils.Scope()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				if filterspan.SkipSpan(a.include, a.exclude, span, resource, library) {
					continue
				}

				host, ok := span.Attributes().Get(conventions.AttributeHTTPHost)
				if !ok {
					continue
				}
				path, ok := span.Attributes().Get("http.path")
				if !ok {
					continue
				}

				k := attributeKey(host.AsString(), path.AsString())
				a.mu.RLock()
				attr, ok := a.attributesCache[k]
				a.mu.RUnlock()
				if !ok {
					a.logger.Info("no attributes found in cache for key", zap.String("key", k))
					if err := a.updateAttributes(host.AsString(), path.AsString()); err != nil {
						a.logger.Error(fmt.Sprintf("error updating attributes: %v", err))
					}
				}

				span.Attributes().InsertString(attrCategories, fmt.Sprintf("%v", attr.categories))
				span.Attributes().InsertString(attrLegalBases, fmt.Sprintf("%v", attr.legalBases))
				span.Attributes().InsertString(attrLegimateInterests, fmt.Sprintf("%v", attr.legitametInterests))
				span.Attributes().InsertString(attrStorages, fmt.Sprintf("%v", attr.storages))
			}
		}
	}
	return td, nil
}

func attributeKey(host, path string) string {
	return fmt.Sprintf("%s/%s", host, path)
}

func (a *transparencyProcessor) updateAttributes(httpHost, httpPath string) error {
	url := path.Clean(fmt.Sprintf("%s/tilt/%s", httpHost, httpPath))
	res, err := http.Get(url)
	if err != nil || res.StatusCode >= 400 {
		return fmt.Errorf("error fetching spec from %q: %v", url, err)
	}
	defer res.Body.Close()
	d := json.NewDecoder(res.Body)
	spec := new(tiltSpec)
	if err := d.Decode(spec); err != nil {
		return fmt.Errorf("error decoding spec from %q: %v", url, err)
	}

	attributes := attributes{}

	for _, d := range spec.DataDisclosed {
		attributes.categories = append(attributes.categories, d.Category)
		for _, l := range d.LegalBases {
			attributes.legalBases = append(attributes.legalBases, l.Reference)
		}
		for _, l := range d.LegitimateInterests {
			attributes.legitametInterests = append(attributes.legitametInterests, l.Exists)
		}
		for _, s := range d.Storage {
			for _, t := range s.Temporal {
				attributes.storages = append(attributes.storages, t.TTL)
			}
		}
	}

	a.mu.Lock()
	a.attributesCache[attributeKey(httpHost, httpPath)] = attributes
	a.mu.Unlock()
	return nil
}
