package common

import (
	"time"

	"github.com/grafana/pyroscope/api/gen/proto/go/fuzz"
	"github.com/grafana/pyroscope/pkg/distributor/ingestlimits"
	"github.com/grafana/pyroscope/pkg/distributor/sampling"
	"github.com/grafana/pyroscope/pkg/validation"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/relabel"
)

func ConvertIngestlimitsConfig(msg *fuzz.IngestlimitsConfig) (ingestlimits.Config, error) {
	if msg == nil {
		return ingestlimits.Config{}, nil
	}
	result := ingestlimits.Config{}
	var err error
	_ = err
	result.PeriodType = msg.PeriodType
	result.PeriodLimitMb = int(msg.PeriodLimitMb)
	result.LimitResetTime = msg.LimitResetTime
	result.LimitReached = msg.LimitReached
	result.Sampling.NumRequests = int(msg.IngestlimitsSamplingConfig__NumRequests)
	result.Sampling.Period = time.Duration(msg.IngestlimitsSamplingConfig__Period)
	if msg.UsageGroups != nil {
		result.UsageGroups = make(map[string]ingestlimits.UsageGroup)
		for k, v := range msg.UsageGroups {
			if v != nil {
				converted, err := ConvertIngestlimitsUsageGroup(v)
				if err == nil {
					result.UsageGroups[k] = converted
				}
			}
		}
	}
	return result, nil
}

func ConvertIngestlimitsUsageGroup(msg *fuzz.IngestlimitsUsageGroup) (ingestlimits.UsageGroup, error) {
	if msg == nil {
		return ingestlimits.UsageGroup{}, nil
	}
	result := ingestlimits.UsageGroup{}
	var err error
	_ = err
	result.PeriodLimitMb = int(msg.PeriodLimitMb)
	result.LimitReached = msg.LimitReached
	return result, nil
}

func ConvertRelabelConfig(msg *fuzz.RelabelConfig) (relabel.Config, error) {
	if msg == nil {
		return relabel.Config{}, nil
	}
	result := relabel.Config{}
	var err error
	_ = err
	if msg.SourceLabels != nil {
		result.SourceLabels = make(model.LabelNames, len(msg.SourceLabels))
		for i, v := range msg.SourceLabels {
			result.SourceLabels[i] = model.LabelName(v)
		}
	}
	result.Separator = msg.Separator
	if msg.Regex != "" {
		regexp, err := relabel.NewRegexp(msg.Regex)
		if err == nil {
			result.Regex = regexp
		}
	}
	result.Modulus = msg.Modulus
	result.TargetLabel = msg.TargetLabel
	result.Replacement = msg.Replacement
	result.Action = relabel.Action(msg.Action)
	err = result.Validate()
	if err != nil {
		return result, err
	}
	return result, nil
}

func ConvertSamplingConfig(msg *fuzz.SamplingConfig) (sampling.Config, error) {
	if msg == nil {
		return sampling.Config{}, nil
	}
	result := sampling.Config{}
	var err error
	_ = err
	if msg.UsageGroups != nil {
		result.UsageGroups = make(map[string]sampling.UsageGroupSampling)
		for k, v := range msg.UsageGroups {
			if v != nil {
				converted, err := ConvertSamplingUsageGroupSampling(v)
				if err == nil {
					result.UsageGroups[k] = converted
				}
			}
		}
	}
	return result, nil
}

func ConvertSamplingUsageGroupSampling(msg *fuzz.SamplingUsageGroupSampling) (sampling.UsageGroupSampling, error) {
	if msg == nil {
		return sampling.UsageGroupSampling{}, nil
	}
	result := sampling.UsageGroupSampling{}
	var err error
	_ = err
	result.Probability = float64(msg.Probability)
	return result, nil
}

func ConvertValidationLimits(msg *fuzz.ValidationLimits) (validation.Limits, error) {
	if msg == nil {
		return validation.Limits{}, nil
	}
	result := validation.Limits{}
	var err error
	_ = err
	result.IngestionRateMB = float64(msg.IngestionRateMB)
	result.IngestionBurstSizeMB = float64(msg.IngestionBurstSizeMB)
	if msg.IngestionLimit != nil {
		converted, err := ConvertIngestlimitsConfig(msg.IngestionLimit)
		if err == nil {
			result.IngestionLimit = &converted
		}
	}
	result.IngestionBodyLimitMB = float64(msg.IngestionBodyLimitMB)
	if msg.DistributorSampling != nil {
		converted, err := ConvertSamplingConfig(msg.DistributorSampling)
		if err == nil {
			result.DistributorSampling = &converted
		}
	}
	result.MaxLabelNameLength = int(msg.MaxLabelNameLength)
	result.MaxLabelValueLength = int(msg.MaxLabelValueLength)
	result.MaxLabelNamesPerSeries = int(msg.MaxLabelNamesPerSeries)
	result.MaxSessionsPerSeries = int(msg.MaxSessionsPerSeries)
	result.EnforceLabelsOrder = msg.EnforceLabelsOrder
	result.MaxProfileSizeBytes = int(msg.MaxProfileSizeBytes)
	result.MaxProfileStacktraceSamples = int(msg.MaxProfileStacktraceSamples)
	result.MaxProfileStacktraceSampleLabels = int(msg.MaxProfileStacktraceSampleLabels)
	result.MaxProfileStacktraceDepth = int(msg.MaxProfileStacktraceDepth)
	result.MaxProfileSymbolValueLength = int(msg.MaxProfileSymbolValueLength)
	if msg.DistributorUsageGroups != nil {
		result.DistributorUsageGroups = &validation.UsageGroupConfig{}
		err = result.DistributorUsageGroups.UnmarshalMap(msg.DistributorUsageGroups)
		if err != nil {
			return result, err
		}
	}
	result.DistributorAggregationWindow = model.Duration(msg.DistributorAggregationWindow)
	result.DistributorAggregationPeriod = model.Duration(msg.DistributorAggregationPeriod)
	if msg.IngestionRelabelingRules != nil {
		result.IngestionRelabelingRules = make([]*relabel.Config, 0, len(msg.IngestionRelabelingRules))
		for _, item := range msg.IngestionRelabelingRules {
			converted, err := ConvertRelabelConfig(item)
			if err == nil {
				err = converted.Validate()
				if err == nil {
					result.IngestionRelabelingRules = append(result.IngestionRelabelingRules, &converted)
				}
			}
		}
	}
	err = result.IngestionRelabelingDefaultRulesPosition.Set(msg.IngestionRelabelingDefaultRulesPosition)
	if err != nil {
		return result, err
	}
	result.IngestionTenantShardSize = int(msg.IngestionTenantShardSize)
	result.IngestionArtificialDelay = model.Duration(msg.IngestionArtificialDelay)
	result.MaxLocalSeriesPerTenant = int(msg.MaxLocalSeriesPerTenant)
	result.MaxGlobalSeriesPerTenant = int(msg.MaxGlobalSeriesPerTenant)
	result.MaxQueryLookback = model.Duration(msg.MaxQueryLookback)
	result.MaxQueryLength = model.Duration(msg.MaxQueryLength)
	result.MaxQueryParallelism = int(msg.MaxQueryParallelism)
	result.QueryAnalysisEnabled = msg.QueryAnalysisEnabled
	result.QueryAnalysisSeriesEnabled = msg.QueryAnalysisSeriesEnabled
	result.MaxFlameGraphNodesDefault = int(msg.MaxFlameGraphNodesDefault)
	result.MaxFlameGraphNodesMax = int(msg.MaxFlameGraphNodesMax)
	result.StoreGatewayTenantShardSize = int(msg.StoreGatewayTenantShardSize)
	result.QuerySplitDuration = model.Duration(msg.QuerySplitDuration)
	result.CompactorBlocksRetentionPeriod = model.Duration(msg.CompactorBlocksRetentionPeriod)
	result.CompactorSplitAndMergeShards = int(msg.CompactorSplitAndMergeShards)
	result.CompactorSplitAndMergeStageSize = int(msg.CompactorSplitAndMergeStageSize)
	result.CompactorSplitGroups = int(msg.CompactorSplitGroups)
	result.CompactorTenantShardSize = int(msg.CompactorTenantShardSize)
	result.CompactorPartialBlockDeletionDelay = model.Duration(msg.CompactorPartialBlockDeletionDelay)
	result.CompactorDownsamplerEnabled = msg.CompactorDownsamplerEnabled
	result.S3SSEType = msg.S3SSEType
	result.S3SSEKMSKeyID = msg.S3SSEKMSKeyID
	result.S3SSEKMSEncryptionContext = msg.S3SSEKMSEncryptionContext
	result.RejectOlderThan = model.Duration(msg.RejectOlderThan)
	result.RejectNewerThan = model.Duration(msg.RejectNewerThan)
	err = result.WritePathOverrides.WritePath.Set(msg.WritepathConfig__WritePath)
	if err != nil {
		return result, err
	}
	result.WritePathOverrides.IngesterWeight = float64(msg.WritepathConfig__IngesterWeight)
	result.WritePathOverrides.SegmentWriterWeight = float64(msg.WritepathConfig__SegmentWriterWeight)
	result.WritePathOverrides.SegmentWriterTimeout = time.Duration(msg.WritepathConfig__SegmentWriterTimeout)
	err = result.WritePathOverrides.Compression.Set(msg.WritepathConfig__Compression)
	if err != nil {
		return result, err
	}
	result.WritePathOverrides.AsyncIngest = msg.WritepathConfig__AsyncIngest
	result.ReadPathOverrides.EnableQueryBackend = msg.ReadpathConfig__EnableQueryBackend
	result.ReadPathOverrides.EnableQueryBackendFrom = time.Unix(0, int64(msg.ReadpathConfig__EnableQueryBackendFrom))
	result.Retention.RetentionPeriod = model.Duration(msg.RetentionConfig__RetentionPeriod)
	result.AdaptivePlacementLimits.TenantShards = msg.AdaptiveplacementPlacementLimits__TenantShards
	result.AdaptivePlacementLimits.DefaultDatasetShards = msg.AdaptiveplacementPlacementLimits__DefaultDatasetShards
	err = result.AdaptivePlacementLimits.LoadBalancing.Set(msg.AdaptiveplacementPlacementLimits__LoadBalancing)
	if err != nil {
		return result, err
	}
	result.AdaptivePlacementLimits.MinDatasetShards = msg.AdaptiveplacementPlacementLimits__MinDatasetShards
	result.AdaptivePlacementLimits.MaxDatasetShards = msg.AdaptiveplacementPlacementLimits__MaxDatasetShards
	result.AdaptivePlacementLimits.UnitSizeBytes = msg.AdaptiveplacementPlacementLimits__UnitSizeBytes
	result.AdaptivePlacementLimits.BurstWindow = time.Duration(msg.AdaptiveplacementPlacementLimits__BurstWindow)
	result.AdaptivePlacementLimits.DecayWindow = time.Duration(msg.AdaptiveplacementPlacementLimits__DecayWindow)
	result.RecordingRules = msg.RecordingRules
	result.Symbolizer.Enabled = msg.ValidationSymbolizer__Enabled
	err = result.Validate()
	if err != nil {
		return result, err
	}
	return result, nil
}
