// SPDX-License-Identifier: ice License 1.0

package metadata

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

type videoMetaExtractor struct{}
type (
	VideoMetadata struct {
		Format  *VideoFormat   `json:"format"`
		Streams []*VideoStream `json:"streams"`
	}
	VideoFormat struct {
		Tags struct {
			MajorBrand       string `json:"major_brand"`
			MinorVersion     string `json:"minor_version"`
			CompatibleBrands string `json:"compatible_brands"`
			Encoder          string `json:"encoder"`
			LocationEng      string `json:"location-eng"`
			Location         string `json:"location"`
		} `json:"tags"`
		Filename       string `json:"filename"`
		FormatName     string `json:"format_name"`
		FormatLongName string `json:"format_long_name"`
		StartTime      string `json:"start_time"`
		Duration       string `json:"duration"`
		Size           string `json:"size"`
		BitRate        string `json:"bit_rate"`
		NbStreams      int    `json:"nb_streams"`
		NbPrograms     int    `json:"nb_programs"`
		ProbeScore     int    `json:"probe_score"`
	}
	VideoStream struct {
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
		} `json:"tags"`
		CodecTimeBase  string `json:"codec_time_base"`
		Duration       string `json:"duration"`
		SampleRate     string `json:"sample_rate"`
		CodecType      string `json:"codec_type"`
		BitRate        string `json:"bit_rate"`
		CodecTagString string `json:"codec_tag_string"`
		CodecTag       string `json:"codec_tag"`
		SampleFmt      string `json:"sample_fmt"`
		Profile        string `json:"profile"`
		CodecLongName  string `json:"codec_long_name"`
		CodecName      string `json:"codec_name"`
		ChannelLayout  string `json:"channel_layout"`
		RFrameRate     string `json:"r_frame_rate"`
		AvgFrameRate   string `json:"avg_frame_rate"`
		TimeBase       string `json:"time_base"`
		NbFrames       string `json:"nb_frames"`
		StartTime      string `json:"start_time"`
		MaxBitRate     string `json:"max_bit_rate"`
		Disposition    struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
		} `json:"disposition"`
		BitsPerSample int `json:"bits_per_sample"`
		DurationTs    int `json:"duration_ts"`
		StartPts      int `json:"start_pts"`
		Width         int `json:"width"`
		Height        int `json:"height"`
		Index         int `json:"index"`
		Channels      int `json:"channels"`
	}
)

func (v *videoMetaExtractor) Close() error {
	return nil
}

func (v *videoMetaExtractor) Extract(filePath, _ string, size uint64) (*Metadata, error) {
	res, err := ffmpeg.Probe(filePath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch video metadata from %s", filePath)
	}
	var md VideoMetadata
	if err = json.Unmarshal([]byte(res), &md); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal video metadata from %s", filePath)
	}
	ext := filepath.Ext(filePath)
	return &Metadata{
		Ext:      ext,
		Size:     size,
		TypeMeta: &md,
	}, nil
}

func newVideoExtractor() Extractor {
	res, err := ffmpeg.Probe("")
	if err != nil || !strings.Contains(res, "You have to specify one input file") {
		if err == nil {
			err = errors.New(res)
		}
		if strings.Contains(err.Error(), "ffprobe version ") {
			err = nil
		}
		if err != nil {
			log.Panic(errors.Wrapf(err, "failed to call ffprobe, is ffmpeg installed?"))
		}
	}
	return &videoMetaExtractor{}
}
