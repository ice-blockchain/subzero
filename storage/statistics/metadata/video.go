package metadata

import (
	"encoding/json"
	"github.com/gookit/goutil/errorx"
	ffmpeg "github.com/u2takey/ffmpeg-go"
	"log"
	"path/filepath"
	"strings"
)

type videoMetaExtractor struct{}
type (
	VideoMetadata struct {
		Format  *VideoFormat   `json:"format"`
		Streams []*VideoStream `json:"streams"`
	}
	VideoFormat struct {
		Filename       string `json:"filename"`
		NbStreams      int    `json:"nb_streams"`
		NbPrograms     int    `json:"nb_programs"`
		FormatName     string `json:"format_name"`
		FormatLongName string `json:"format_long_name"`
		StartTime      string `json:"start_time"`
		Duration       string `json:"duration"`
		Size           string `json:"size"`
		BitRate        string `json:"bit_rate"`
		ProbeScore     int    `json:"probe_score"`
		Tags           struct {
			MajorBrand       string `json:"major_brand"`
			MinorVersion     string `json:"minor_version"`
			CompatibleBrands string `json:"compatible_brands"`
			Encoder          string `json:"encoder"`
			LocationEng      string `json:"location-eng"`
			Location         string `json:"location"`
		} `json:"tags"`
	}
	VideoStream struct {
		Index          int    `json:"index"`
		CodecName      string `json:"codec_name"`
		CodecLongName  string `json:"codec_long_name"`
		Profile        string `json:"profile"`
		CodecType      string `json:"codec_type"`
		CodecTimeBase  string `json:"codec_time_base"`
		CodecTagString string `json:"codec_tag_string"`
		CodecTag       string `json:"codec_tag"`
		SampleFmt      string `json:"sample_fmt"`
		SampleRate     string `json:"sample_rate"`
		Channels       int    `json:"channels"`
		ChannelLayout  string `json:"channel_layout"`
		BitsPerSample  int    `json:"bits_per_sample"`
		RFrameRate     string `json:"r_frame_rate"`
		AvgFrameRate   string `json:"avg_frame_rate"`
		TimeBase       string `json:"time_base"`
		StartPts       int    `json:"start_pts"`
		StartTime      string `json:"start_time"`
		DurationTs     int    `json:"duration_ts"`
		Duration       string `json:"duration"`
		BitRate        string `json:"bit_rate"`
		MaxBitRate     string `json:"max_bit_rate"`
		NbFrames       string `json:"nb_frames"`
		Width          int    `json:"width"`
		Height         int    `json:"height"`
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
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
		} `json:"tags"`
	}
)

func (v *videoMetaExtractor) Close() error {
	return nil
}

func (v *videoMetaExtractor) Extract(filePath, _ string, size uint64) (*Metadata, error) {
	res, err := ffmpeg.Probe(filePath)
	if err != nil {
		return nil, errorx.Withf(err, "failed to fetch video metadata from %s", filePath)
	}
	var md VideoMetadata
	if err = json.Unmarshal([]byte(res), &md); err != nil {
		return nil, errorx.Withf(err, "failed to unmarshal video metadata from %s", filePath)
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
			err = errorx.New(res)
		}
		if strings.Contains(err.Error(), "ffprobe version ") {
			err = nil
		}
		if err != nil {
			log.Panic(errorx.Withf(err, "failed to call ffprobe, is ffmpeg installed?"))
		}
	}
	return &videoMetaExtractor{}
}
