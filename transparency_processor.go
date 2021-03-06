package transparencyprocessor

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mindtastic/opentelemetry-transparency-processor/internal/filterspan"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.uber.org/zap"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

var processorCapabilities = consumer.Capabilities{MutatesData: true}

const (
	attrCategories          = "tilt.categories"
	attrLegalBases          = "tilt.legal_bases"
	attrLegitimateInterests = "tilt.legitimate_interests"
	attrStorages            = "tilt.storage_durations"
	attrPurposes            = "tilt.purposes"
	attrAutomatedDecision   = "tilt.automated_decision_making"
)

type tiltAttributes struct {
	lastUpdated        time.Time
	categories         []string
	legalBases         []string
	legitametInterests []bool
	storages           []string
	puproses           []string
	automatedDecision  bool
}

type transparencyProcessor struct {
	logger    *zap.Logger
	exportCtx context.Context

	timeout time.Duration

	telemetryLevel configtelemetry.Level

	serviceMap map[string]string

	mu              sync.RWMutex
	attributesCache map[string]tiltAttributes
	include         filterspan.Matcher
	exclude         filterspan.Matcher
	//attrProc        *attraction.AttrProc
}

func newTransparencyProcessor(set component.ProcessorCreateSettings, include, exclude filterspan.Matcher, serviceMap map[string]string) *transparencyProcessor {
	tp := new(transparencyProcessor)
	tp.logger = set.Logger
	tp.attributesCache = make(map[string]tiltAttributes)
	tp.mu = sync.RWMutex{}
	tp.serviceMap = serviceMap
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

				// Overwrite "linkerd-proxy" to the actual component name
				component, ok := resource.Attributes().Get("linkerd.io/proxy-deployment")
				if !ok {
					continue
				}
				p := pcommon.NewValueString(fmt.Sprintf("%s-proxy", component.AsString()))
				resource.Attributes().Update(conventions.AttributeServiceName, p)

				tHost, ok := span.Attributes().Get(conventions.AttributeHTTPHost)
				if !ok {
					tHost, ok = resource.Attributes().Get(conventions.AttributeHTTPHost)
					if !ok {
						continue
					}
				}

				k := attributeKey(tHost.AsString(), span.Name())
				a.mu.RLock()
				attr, ok := a.attributesCache[k]
				a.mu.RUnlock()
				if !ok {
					a.logger.Info("no tiltAttributes found in cache for key", zap.String("key", k))
					attributes, err := a.updateAttributes(tHost.AsString(), span.Name())
					if err != nil {
						a.logger.Warn(fmt.Sprintf("error updating tiltAttributes: %v", err))
					}
					attr = attributes
				}

				insertAttributes(span, attrCategories, attr.categories)
				insertAttributes(span, attrLegalBases, attr.legalBases)
				insertAttributes(span, attrStorages, attr.storages)
				insertAttributes(span, attrPurposes, attr.puproses)
				if attr.automatedDecision {
					span.Attributes().InsertBool(attrAutomatedDecision, attr.automatedDecision)
				}
				span.Attributes().InsertString(attrLegitimateInterests, fmt.Sprintf("%v", attr.legitametInterests))
			}
		}
	}
	return td, nil
}

func insertAttributes(span ptrace.Span, key string, values []string) {
	if len(values) == 0 {
		return
	}
	b := pcommon.NewSlice()
	b.EnsureCapacity(len(values))
	for _, c := range values {
		v := b.AppendEmpty()
		v.SetStringVal(c)
	}
	vs := pcommon.NewValueSlice()
	b.CopyTo(vs.SliceVal())
	span.Attributes().Insert(key, vs)
}

func attributeKey(httHost, httpPath string) string {
	return path.Clean(fmt.Sprintf("%s/%s", httHost, httpPath))
}

func (a *transparencyProcessor) updateAttributes(httpHost, httpPath string) (tiltAttributes, error) {
	host, ok := a.serviceMap[httpHost]
	if !ok {
		host = httpHost
	}
	if !strings.Contains(httpPath, "tilt/") {
		httpPath = "tilt/" + httpPath
	}
	u := url.URL{
		Scheme: "http",
		Host:   host,
		Path:   path.Clean(httpPath),
	}

	res, err := http.Get(u.String())
	if err != nil || res.StatusCode >= 400 {
		a.mu.Lock()
		a.attributesCache[attributeKey(httpHost, httpPath)] = tiltAttributes{lastUpdated: time.Now()}
		a.mu.Unlock()
		return tiltAttributes{}, fmt.Errorf("error fetching spec from %q: %v", u.String(), err)
	}
	defer res.Body.Close()
	d := json.NewDecoder(res.Body)
	spec := new(tiltSpec)
	if err := d.Decode(spec); err != nil {
		return tiltAttributes{}, fmt.Errorf("error decoding spec from %q: %v", u.String(), err)
	}

	attributes := tiltAttributes{}

	for _, d := range spec.DataDisclosed {
		attributes.categories = append(attributes.categories, d.Category)
		for _, l := range d.LegalBases {
			attributes.legalBases = append(attributes.legalBases, l.Reference)
		}
		for _, p := range d.Purposes {
			attributes.puproses = append(attributes.puproses, p.Purpose)
		}
		for _, l := range d.LegitimateInterests {
			attributes.legitametInterests = append(attributes.legitametInterests, l.Exists)
		}
		for _, s := range d.Storage {
			for _, t := range s.Temporal {
				attributes.storages = append(attributes.storages, t.TTL)
			}
		}
		attributes.automatedDecision = spec.AutomatedDecisionMaking.InUse
	}

	a.mu.Lock()
	a.attributesCache[attributeKey(httpHost, httpPath)] = attributes
	a.mu.Unlock()
	return attributes, nil
}
