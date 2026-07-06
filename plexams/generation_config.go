package plexams

import (
	"context"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/obcode/plexams.go/plexams/invigplan"
	"github.com/spf13/viper"
)

// GenerationConfig returns the global generation config. When none is stored yet it
// falls back to the package defaults, seeded from the config file (viper) for
// backwards compatibility.
func (p *Plexams) GenerationConfig(ctx context.Context) (*model.GenerationConfig, error) {
	cfg, err := p.dbClient.GetGenerationConfig(ctx)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		fillSlotTimeDefaults(cfg) // backfill fields absent from an older stored config
		return cfg, nil
	}
	return defaultGenerationConfig(), nil
}

// fillSlotTimeDefaults sets sensible defaults for the start-time-avoidance fields when a
// stored config predates them (so an existing DB keeps the AUTO-by-semester behaviour
// instead of decoding to a zero-valued, effectively-OFF config).
func fillSlotTimeDefaults(cfg *model.GenerationConfig) {
	if cfg.SlotTimeMode == "" {
		cfg.SlotTimeMode = model.SlotTimeConstraintModeAuto
	}
	if cfg.SlotTimeWeight == 0 {
		cfg.SlotTimeWeight = defaultSlotTimeWeight
	}
	if cfg.SlotTimeWinterEarliest == "" {
		cfg.SlotTimeWinterEarliest = defaultSlotTimeWinterEarliest
	}
}

// SetGenerationConfig stores the global generation config.
func (p *Plexams) SetGenerationConfig(ctx context.Context, cfg *model.GenerationConfig) (*model.GenerationConfig, error) {
	if err := p.dbClient.SetGenerationConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// defaultGenerationConfig builds the generation config from the invigplan defaults,
// overlaid with any values still present in the config file (legacy seed).
func defaultGenerationConfig() *model.GenerationConfig {
	opts := invigplan.DefaultOptions()
	w := invigplan.DefaultWeights()

	cfg := &model.GenerationConfig{
		TimelagMin:             15,
		Iterations:             opts.Iterations,
		StartTemp:              opts.StartTemp,
		EndTemp:                opts.EndTemp,
		ToleranceMin:           60,
		MaxSpanHours:           0,
		WeightMinuteBalance:    w.MinuteBalance,
		WeightBeyondTolerance:  w.BeyondTolerance,
		WeightOverTargetFactor: w.OverTargetFactor,
		WeightCoverage:         w.Coverage,
		WeightMaxDays:          w.MaxDays,
		WeightPreferExamDays:   w.PreferExamDays,
		WeightDistribution:     w.Distribution,
		WeightDaySpan:          w.DaySpan,
		SlotTimeMode:           model.SlotTimeConstraintModeAuto,
		SlotTimeWeight:         defaultSlotTimeWeight,
		SlotTimeWinterEarliest: defaultSlotTimeWinterEarliest,
	}

	// legacy seed from the config file
	if viper.IsSet("rooms.timelag") {
		cfg.TimelagMin = viper.GetInt("rooms.timelag")
	}
	if viper.IsSet("invigilation.optimizer.iterations") {
		cfg.Iterations = viper.GetInt("invigilation.optimizer.iterations")
	}
	if viper.IsSet("invigilation.optimizer.startTemp") {
		cfg.StartTemp = viper.GetFloat64("invigilation.optimizer.startTemp")
	}
	if viper.IsSet("invigilation.optimizer.endTemp") {
		cfg.EndTemp = viper.GetFloat64("invigilation.optimizer.endTemp")
	}
	if viper.IsSet("invigilation.optimizer.tolerance") {
		cfg.ToleranceMin = viper.GetInt("invigilation.optimizer.tolerance")
	}
	if viper.IsSet("invigilation.optimizer.maxSpanHours") {
		cfg.MaxSpanHours = viper.GetFloat64("invigilation.optimizer.maxSpanHours")
	}
	if viper.IsSet("invigilation.optimizer.weights.minuteBalance") {
		cfg.WeightMinuteBalance = viper.GetFloat64("invigilation.optimizer.weights.minuteBalance")
	}
	if viper.IsSet("invigilation.optimizer.weights.beyondTolerance") {
		cfg.WeightBeyondTolerance = viper.GetFloat64("invigilation.optimizer.weights.beyondTolerance")
	}
	if viper.IsSet("invigilation.optimizer.weights.overTargetFactor") {
		cfg.WeightOverTargetFactor = viper.GetFloat64("invigilation.optimizer.weights.overTargetFactor")
	}
	if viper.IsSet("invigilation.optimizer.weights.coverage") {
		cfg.WeightCoverage = viper.GetFloat64("invigilation.optimizer.weights.coverage")
	}
	if viper.IsSet("invigilation.optimizer.weights.maxDays") {
		cfg.WeightMaxDays = viper.GetFloat64("invigilation.optimizer.weights.maxDays")
	}
	if viper.IsSet("invigilation.optimizer.weights.preferExamDays") {
		cfg.WeightPreferExamDays = viper.GetFloat64("invigilation.optimizer.weights.preferExamDays")
	}
	if viper.IsSet("invigilation.optimizer.weights.distribution") {
		cfg.WeightDistribution = viper.GetFloat64("invigilation.optimizer.weights.distribution")
	}
	if viper.IsSet("invigilation.optimizer.weights.daySpan") {
		cfg.WeightDaySpan = viper.GetFloat64("invigilation.optimizer.weights.daySpan")
	}

	return cfg
}

// generationTimelagMin returns the configured room time lag (minutes), falling back
// to the default on error.
func (p *Plexams) generationTimelagMin(ctx context.Context) int {
	cfg, err := p.GenerationConfig(ctx)
	if err != nil {
		return defaultGenerationConfig().TimelagMin
	}
	return cfg.TimelagMin
}

// optimizerWeights maps the generation config to invigplan.Weights.
func optimizerWeights(cfg *model.GenerationConfig) invigplan.Weights {
	return invigplan.Weights{
		MinuteBalance:    cfg.WeightMinuteBalance,
		BeyondTolerance:  cfg.WeightBeyondTolerance,
		Coverage:         cfg.WeightCoverage,
		MaxDays:          cfg.WeightMaxDays,
		PreferExamDays:   cfg.WeightPreferExamDays,
		Distribution:     cfg.WeightDistribution,
		DaySpan:          cfg.WeightDaySpan,
		OverTargetFactor: cfg.WeightOverTargetFactor,
	}
}
