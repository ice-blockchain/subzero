// SPDX-License-Identifier: ice License 1.0

package eventmatcher

import (
	"github.com/RoaringBitmap/roaring/v2/roaring64"
)

type (
	// matcherBitmap wraps a roaring.Bitmap with lazy initialization.
	matcherBitmap struct {
		*roaring64.Bitmap
	}
)

func (bm *matcherBitmap) IsEmpty() bool {
	return bm.Bitmap == nil || bm.Bitmap.IsEmpty()
}

func (bm *matcherBitmap) GetCardinality() uint64 {
	if bm.Bitmap == nil {
		return 0
	}
	return bm.Bitmap.GetCardinality()
}

func (bm *matcherBitmap) Add(x uint64) *matcherBitmap {
	if bm.Bitmap == nil {
		bm.Bitmap = roaring64.New()
	}
	bm.Bitmap.Add(x)
	return bm
}

func (bm *matcherBitmap) Remove(x uint64) *matcherBitmap {
	if bm.Bitmap != nil {
		bm.Bitmap.Remove(x)
	}
	return bm
}

func (bm matcherBitmap) String() string {
	if bm.Bitmap == nil {
		return "{nil}"
	}
	return bm.Bitmap.String()
}
