package transparencyprocessor

import (
	"context"
	"github.com/mindtastic/opentelemetry-transparency-processor/internal/filterspan"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

const typeStr = "transparency"

func NewFactory() component.ProcessorFactory {
	return component.NewProcessorFactory(
		typeStr,
		createDefaultConfig,
		component.WithTracesProcessor(createTracesProcessor),
	)
}

func createDefaultConfig() config.Processor {
	return &Config{
		ProcessorSettings: config.NewProcessorSettings(config.NewComponentID(typeStr)),
	}
}

func createTracesProcessor(_ context.Context, set component.ProcessorCreateSettings, cfg config.Processor, nextConsumer consumer.Traces) (component.TracesProcessor, error) {
	oCfg := cfg.(*Config)
	include, err := filterspan.NewMatcher(oCfg.Include)
	if err != nil {
		return nil, err
	}
	exclude, err := filterspan.NewMatcher(oCfg.Exclude)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTracesProcessor(
		cfg, nextConsumer,
		newTransparencyProcessor(set, include, exclude).processTraces,
		processorhelper.WithCapabilities(processorCapabilities),
	)
}
