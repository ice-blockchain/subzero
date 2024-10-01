// SPDX-License-Identifier: ice License 1.0

package statistics

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gookit/goutil/errorx"
	"github.com/rcrowley/go-metrics"

	"github.com/ice-blockchain/subzero/storage/statistics/metadata"
)

type (
	Statistics interface {
		io.Closer
		ProcessFile(filePath, contentType string, size uint64)
	}
	statistics struct {
		metaExtractor  metadata.Extractor
		metrics        metrics.Registry
		rootStorageDir string
	}
	noopStats struct{}
)

func (n *noopStats) Close() error {
	return nil
}

func (n *noopStats) ProcessFile(filePath, contentType string, size uint64) {
}

const (
	imageWidth   = "imageWidth"
	imageHeight  = "imageHeight"
	fileSize     = "fileSize"
	duration     = "duration"
	videoWidth   = "videoWidth"
	videoHeight  = "videoHeight"
	videoBitrate = "videoBitrate"
	audioBitrate = "audioBitrate"
)

func NewStatistics(rootStorageDir string, debug bool) Statistics {
	if !debug {
		return &noopStats{}
	}
	s := &statistics{
		metaExtractor:  metadata.NewExtractor(),
		metrics:        metrics.NewRegistry(),
		rootStorageDir: rootStorageDir,
	}
	if err := s.metrics.Register(imageWidth, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", imageWidth))
	}
	if err := s.metrics.Register(imageHeight, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", imageHeight))
	}
	if err := s.metrics.Register(videoWidth, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", videoWidth))
	}
	if err := s.metrics.Register(videoHeight, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", videoHeight))
	}
	if err := s.metrics.Register(duration, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", duration))
	}
	if err := s.metrics.Register(videoBitrate, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", videoBitrate))
	}
	if err := s.metrics.Register(audioBitrate, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", audioBitrate))
	}
	if err := s.metrics.Register(fileSize, metrics.NewHistogram(metrics.NewExpDecaySample(10000, 0.15))); err != nil {
		log.Panic(errorx.Withf(err, "failed to register metric %v", fileSize))
	}
	go func() {
		for _ = range time.Tick(60 * time.Second) {
			s.writeJSON()
		}
	}()
	return s
}

func (s *statistics) writeJSON() {
	statsFile, err := os.OpenFile(filepath.Join(s.rootStorageDir, "stats.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		log.Printf("ERROR: %v", errorx.Withf(err, "failed to open file for stats collection"))
	}
	defer func() {
		statsFile.Sync()
		statsFile.Close()
	}()
	metrics.WriteJSONOnce(s.metrics, statsFile)

}

func (s *statistics) Close() error {
	s.writeJSON()
	if err := s.metaExtractor.Close(); err != nil {
		return errorx.Withf(err, "failed to close metadata extractors")
	}
	return nil

}
func (s *statistics) ProcessFile(filePath, contentType string, size uint64) {
	go func() {
		md, err := s.metaExtractor.Extract(filePath, contentType, size)
		if err != nil {
			log.Printf("Error extracting metadata for file stats: %v\n", err)
		}
		s.registerMetadataStats(md)
	}()
}

func (s *statistics) registerMetadataStats(md *metadata.Metadata) {
	s.registerFileStats(md)
	switch typedMD := md.TypeMeta.(type) {
	case *metadata.ImageMetadata:
		s.registerImageStats(md, typedMD)
	case *metadata.VideoMetadata:
		s.registerVideoStats(md, typedMD)
	}

}
func (s *statistics) registerImageStats(md *metadata.Metadata, imageMetadata *metadata.ImageMetadata) {
	s.metrics.Get(imageWidth).(metrics.Histogram).Update(int64(imageMetadata.Width))
	s.metrics.Get(imageHeight).(metrics.Histogram).Update(int64(imageMetadata.Height))
}
func (s *statistics) registerVideoStats(md *metadata.Metadata, videoMetadata *metadata.VideoMetadata) {
	for _, stream := range videoMetadata.Streams {
		switch stream.CodecType {
		case "video":
			s.metrics.Get(videoWidth).(metrics.Histogram).Update(int64(stream.Width))
			s.metrics.Get(videoHeight).(metrics.Histogram).Update(int64(stream.Height))
			dur, err := strconv.ParseFloat(stream.Duration, 64)
			if err == nil {
				s.metrics.Get(duration).(metrics.Histogram).Update(int64(dur))
			}
			bitrate, err := strconv.ParseInt(stream.BitRate, 10, 64)
			if err == nil {
				s.metrics.Get(videoBitrate).(metrics.Histogram).Update(bitrate)
			}
			s.metrics.GetOrRegister(fmt.Sprintf("videoCodec/%v/%v", stream.CodecName, stream.CodecTagString), metrics.NewCounter()).(metrics.Counter).Inc(1)
		case "audio":
			bitrate, err := strconv.ParseInt(stream.BitRate, 10, 64)
			if err == nil {
				s.metrics.Get(audioBitrate).(metrics.Histogram).Update(bitrate)
			}
			s.metrics.GetOrRegister(fmt.Sprintf("audioCodec/%v/%v", stream.CodecName, stream.CodecTagString), metrics.NewCounter()).(metrics.Counter).Inc(1)
		}
	}
}
func (s *statistics) registerFileStats(md *metadata.Metadata) {
	s.metrics.Get(fileSize).(metrics.Histogram).Update(int64(md.Size))
	s.metrics.GetOrRegister("ext/"+md.Ext, metrics.NewCounter()).(metrics.Counter).Inc(1)
}
