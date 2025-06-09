// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"github.com/cockroachdb/errors"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/model"
)

var (
	jobFeedbackStatusValues = map[string]struct{}{
		model.JobFeedbackStatusPaymentRequired: {},
		model.JobFeedbackStatusProcessing:      {},
		model.JobFeedbackStatusError:           {},
		model.JobFeedbackStatusSuccess:         {},
		model.JobFeedbackStatusPartial:         {},
	}
)

func validateKindTextExtractionJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5301 job text extraction: %+v", e)
}

func validateKindSummarizationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job summarization: %+v", e)
}

func validateKindTranslationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5303 job translation: %+v", e)
}

func validateKindTextGenerationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5304 job text generation: %+v", e)
}

func validateKindImageGenerationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5305 job image generation: %+v", e)
}

func validateKindVideoConversionJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5306 job video conversion: %+v", e)
}

func validateKindVideoTranslationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5307 job video translation: %+v", e)
}

func validateKindImageToVideoConversionJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5308 job image to video conversion: %+v", e)
}

func validateKindTextToSpeechGenerationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5309 job text to speech generation: %+v", e)
}

func validateKindNostrContentDiscoveryJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5301 job nostr content discovery: %+v", e)
}

func validateKindNostrPeopleDiscoveryJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job nostr people discovery: %+v", e)
}

func validateKindNostrContentSearchJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5302 job nostr content search: %+v", e)
}

func validateKindNostrPeopleSearchJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5303 job nostr people search: %+v", e)
}

func validateKindNostrEventCountJob(e *model.Event) error {
	if len(e.Tags) == 0 {
		return errors.Wrapf(ErrWrongEventParams, "kind:5400 job nostr event count, no tags: %+v", e)
	}
	if e.Content == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:5400 job nostr event count, no content: %+v", e)
	}
	for _, tag := range e.Tags {
		if tag.Key() == "param" && tag.Value() == "relay" && !nostr.IsValidRelayURL(tag[2]) {
			return errors.Wrapf(ErrWrongEventParams, "kind:5400 wrong relay tag param: %+v", e)
		}
	}

	return nil
}

func validateKindMalwareScanningJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5401 job malware scanning: %+v", e)
}

func validateKindNostrEventTimeStampingJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5402 job nostr event timestamping: %+v", e)
}

func validateKindOpReturnCreationJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5901 job nostr event timestamping: %+v", e)
}

func validateKindNostrEventPublishScheduleJob(e *model.Event) error {
	return errors.Wrapf(ErrUnsupportedJob, "kind:5902 job nostr event publish schedule: %+v", e)
}

func validateKindJobResult(e *model.Event) error {
	if e.Content == "" {
		return errors.Wrap(ErrWrongEventParams, "kind:6xxx job result: content is empty")
	}
	if jobRequestTag := e.GetTag("request"); jobRequestTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:6xxx job result: no job request tag or it is empty: %+v", jobRequestTag)
	}
	if jobRequestIDTag := e.GetTag("e"); jobRequestIDTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:6xxx job result: no job request ID tag or it is empty: %+v", jobRequestIDTag)
	}
	if customerPubkeyTag := e.GetTag("p"); customerPubkeyTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "kind:6xxx job result, no customer pubkey tag or it is empty: %+v", customerPubkeyTag)
	}

	return nil
}

func validateKindFeedbackJob(e *model.Event) error {
	statusTag := e.Tags.GetFirst([]string{"status"})
	if statusTag == nil || len(*statusTag) < 2 {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, no status tag: %+v", e)
	}
	if _, ok := jobFeedbackStatusValues[statusTag.Value()]; !ok {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, wrong status tag: %+v", e)
	}
	jobRequestIDTag := e.Tags.GetFirst([]string{"e"})
	if jobRequestIDTag == nil || len(*jobRequestIDTag) != 2 {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, no job request ID tag: %+v", e)
	}
	customerPubkeyTag := e.Tags.GetFirst([]string{"p"})
	if customerPubkeyTag == nil || len(*customerPubkeyTag) != 2 {
		return errors.Wrapf(ErrWrongEventParams, "kind:7000 job feedback, no customer pubkey tag: %+v", e)
	}

	return nil
}
