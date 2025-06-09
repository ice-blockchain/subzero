// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/model"
)

func validateKindReportEvent(e *model.Event) error {
	if err := validateLabelTags(e); err != nil {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, wrong label tags: %+v", e)
	}
	pTag := e.Tags.GetFirst([]string{"p"})
	if pTag == nil || pTag.Value() == "" {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, missing p tag: %+v", e)
	}
	eTag := e.Tags.GetFirst([]string{"e"})
	if eTag != nil && (len(*eTag) < 3 || !reportTypeSupported((*eTag)[2]) || len(*pTag) > 2) {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, wrong e tag report type value/wrong p tag report type value: %+v", e)
	}
	if eTag == nil && (len(*pTag) < 3 || !reportTypeSupported((*pTag)[2])) {
		return errors.Wrapf(ErrWrongEventParams, "nip-56, wrong p tag report type value:%+v", e)
	}

	return nil
}

func reportTypeSupported(reportType string) bool {
	return reportType == "" || reportType == model.TagReportTypeNudity || reportType == model.TagReportTypeMalware ||
		reportType == model.TagReportTypeProfanity || reportType == model.TagReportTypeIllegal || reportType == model.TagReportTypeSpam ||
		reportType == model.TagReportTypeImpersonation || reportType == model.TagReportTypeOther
}
