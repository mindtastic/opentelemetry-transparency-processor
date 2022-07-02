// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transparencyprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.6.1"

	"github.com/mindtastic/opentelemtry-transparency-processor/internal/filterset"
)

// Common structure for all the Tests
type testCase struct {
	skip               bool
	name               string
	serviceName        string
	inputAttributes    map[string]interface{}
	expectedAttributes map[string]interface{}
}

// runIndividualTestCase is the common logic of passing trace data through a configured attributes processor.
func runIndividualTestCase(t *testing.T, tt testCase, tp component.TracesProcessor) {
	t.Run(tt.name, func(t *testing.T) {
		td := generateTraceData(tt.serviceName, tt.name, tt.inputAttributes)
		assert.NoError(t, tp.ConsumeTraces(context.Background(), td))
		// Ensure that the modified `td` has the attributes sorted:
		sortAttributes(td)
		require.Equal(t, generateTraceData(tt.serviceName, tt.name, tt.expectedAttributes), td)
	})
}

func generateTraceData(serviceName, spanName string, attrs map[string]interface{}) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	if serviceName != "" {
		rs.Resource().Attributes().UpsertString(conventions.AttributeServiceName, serviceName)
	}
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName(spanName)
	pcommon.NewMapFromRaw(attrs).CopyTo(span.Attributes())
	span.Attributes().Sort()
	return td
}

func sortAttributes(td ptrace.Traces) {
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		rs.Resource().Attributes().Sort()
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			spans := ilss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				spans.At(k).Attributes().Sort()
			}
		}
	}
}

func TestProcessTraces(t *testing.T) {
	testCases := []testCase{
		{
			skip:               false,
			name:               "do nothing",
			serviceName:        "testing",
			inputAttributes:    map[string]interface{}{},
			expectedAttributes: map[string]interface{}{},
		},
		{
			skip:               false,
			name:               "do nothing without host and path",
			serviceName:        "linkerd-proxy",
			inputAttributes:    map[string]interface{}{},
			expectedAttributes: map[string]interface{}{},
		},
		{
			name:        "add attributes",
			serviceName: "linkerd-proxy",
			inputAttributes: map[string]interface{}{
				"http.host": "testHost",
				"http.path": "testPath",
			},
			expectedAttributes: map[string]interface{}{
				"http.host":       "testHost",
				"http.path":       "testPath",
				"tilt.categories": "testing",
			},
		},
	}

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	tp, err := factory.CreateTracesProcessor(context.Background(), componenttest.NewNopProcessorCreateSettings(), cfg, consumertest.NewNop())
	require.Nil(t, err)
	require.NotNil(t, tp)

	for _, tt := range testCases {
		if tt.skip {
			continue
		}
		runIndividualTestCase(t, tt, tp)
	}
}

func BenchmarkProcessTraces(b *testing.B) {
	testCases := []testCase{
		{
			name:            "apply_to_span_with_no_attrs",
			inputAttributes: map[string]interface{}{},
			expectedAttributes: map[string]interface{}{
				"attribute1": 123,
			},
		},
		{
			name: "apply_to_span_with_attr",
			inputAttributes: map[string]interface{}{
				"NoModification": false,
			},
			expectedAttributes: map[string]interface{}{
				"attribute1":     123,
				"NoModification": false,
			},
		},
		{
			name:               "dont_apply",
			inputAttributes:    map[string]interface{}{},
			expectedAttributes: map[string]interface{}{},
		},
	}

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	tp, err := factory.CreateTracesProcessor(context.Background(), componenttest.NewNopProcessorCreateSettings(), cfg, consumertest.NewNop())
	require.Nil(b, err)
	require.NotNil(b, tp)

	for _, tt := range testCases {
		td := generateTraceData(tt.serviceName, tt.name, tt.inputAttributes)

		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				assert.NoError(b, tp.ConsumeTraces(context.Background(), td))
			}
		})

		// Ensure that the modified `td` has the attributes sorted:
		sortAttributes(td)
		require.Equal(b, generateTraceData(tt.serviceName, tt.name, tt.expectedAttributes), td)
	}
}

func createConfig(matchType filterset.MatchType) *filterset.Config {
	return &filterset.Config{
		MatchType: matchType,
	}
}
