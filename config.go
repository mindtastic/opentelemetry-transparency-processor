package transparencyprocessor

import (
	"github.com/mindtastic/opentelemetry-transparency-processor/internal/filterconfig"
	"go.opentelemetry.io/collector/config"
)

type Config struct {
	config.ProcessorSettings `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct
	filterconfig.MatchConfig `mapstructure:",squash"`
	ServiceMap               map[string]string `mapstructure:",sqaush"`
}

var _ config.Processor = (*Config)(nil)
