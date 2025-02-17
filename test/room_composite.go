//go:build integration

package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/livekit/egress/pkg/types"
	"github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/rpc"
	"github.com/livekit/protocol/utils"
)

func runRoomCompositeTests(t *testing.T, conf *TestConfig) {
	if !conf.runRoomTests {
		return
	}

	conf.sourceFramerate = 30

	if conf.runFileTests {
		t.Run("RoomComposite/File", func(t *testing.T) {
			testRoomCompositeFile(t, conf)
		})
	}

	if conf.runStreamTests {
		t.Run("RoomComposite/Stream", func(t *testing.T) {
			testRoomCompositeStream(t, conf)
		})
	}

	if conf.runSegmentTests {
		t.Run("RoomComposite/Segments", func(t *testing.T) {
			testRoomCompositeSegments(t, conf)
		})
	}

	if conf.runMultiTests {
		runWebTest(t, conf, "RoomComposite/Multi", types.MimeTypeOpus, types.MimeTypeVP8, func(t *testing.T) {
			testRoomCompositeMulti(t, conf)
		})
	}
}

func testRoomCompositeFile(t *testing.T, conf *TestConfig) {
	for _, test := range []*testCase{
		{
			name:     "h264-high-mp4",
			fileType: livekit.EncodedFileType_MP4,
			options: &livekit.EncodingOptions{
				AudioCodec: livekit.AudioCodec_AAC,
				VideoCodec: livekit.VideoCodec_H264_HIGH,
			},
			filename:               "r_{room_name}_high_{time}.mp4",
			expectVideoTranscoding: true,
		},
		{
			name:                   "h264-high-mp4-limit",
			fileType:               livekit.EncodedFileType_MP4,
			preset:                 livekit.EncodingOptionsPreset_PORTRAIT_H264_720P_30,
			filename:               "r_limit_{time}.mp4",
			sessionTimeout:         time.Second * 20,
			expectVideoTranscoding: true,
		},
		{
			name:      "opus-ogg",
			fileType:  livekit.EncodedFileType_OGG,
			audioOnly: true,
			options: &livekit.EncodingOptions{
				AudioCodec: livekit.AudioCodec_OPUS,
			},
			filename:               "r_{room_name}_opus_{time}",
			expectVideoTranscoding: false,
		},
	} {
		runWebTest(t, conf, test.name, types.MimeTypeOpus, types.MimeTypeH264, func(t *testing.T) {
			fileOutput := &livekit.EncodedFileOutput{
				FileType: test.fileType,
				Filepath: getFilePath(conf.ServiceConfig, test.filename),
			}
			if conf.S3Upload != nil {
				fileOutput.Filepath = test.filename
				fileOutput.Output = &livekit.EncodedFileOutput_S3{
					S3: conf.S3Upload,
				}
			}

			roomRequest := &livekit.RoomCompositeEgressRequest{
				RoomName:    conf.room.Name(),
				Layout:      "speaker-dark",
				AudioOnly:   test.audioOnly,
				FileOutputs: []*livekit.EncodedFileOutput{fileOutput},
			}
			if test.options != nil {
				roomRequest.Options = &livekit.RoomCompositeEgressRequest_Advanced{
					Advanced: test.options,
				}
			} else if test.preset != 0 {
				roomRequest.Options = &livekit.RoomCompositeEgressRequest_Preset{
					Preset: test.preset,
				}
			}

			req := &rpc.StartEgressRequest{
				EgressId: utils.NewGuid(utils.EgressPrefix),
				Request: &rpc.StartEgressRequest_RoomComposite{
					RoomComposite: roomRequest,
				},
			}

			runFileTest(t, conf, req, test)
		})
		if conf.Short {
			return
		}
	}
}

func testRoomCompositeStream(t *testing.T, conf *TestConfig) {
	for _, test := range []*testCase{
		{
			name:                   "room-rtmp",
			expectVideoTranscoding: true,
		},
		{
			name:                   "room-rtmp-limit",
			sessionTimeout:         time.Second * 20,
			expectVideoTranscoding: true,
		},
	} {
		runWebTest(t, conf, test.name, types.MimeTypeOpus, types.MimeTypeVP8, func(t *testing.T) {
			req := &rpc.StartEgressRequest{
				EgressId: utils.NewGuid(utils.EgressPrefix),
				Request: &rpc.StartEgressRequest_RoomComposite{
					RoomComposite: &livekit.RoomCompositeEgressRequest{
						RoomName: conf.room.Name(),
						Layout:   "grid-light",
						StreamOutputs: []*livekit.StreamOutput{{
							Protocol: livekit.StreamProtocol_RTMP,
							Urls:     []string{streamUrl1},
						}},
					},
				},
			}

			runStreamTest(t, conf, req, test)
		})
		if conf.Short {
			return
		}
	}

	runWebTest(t, conf, "rtmp-bad-url", types.MimeTypeOpus, types.MimeTypeVP8, func(t *testing.T) {
		req := &rpc.StartEgressRequest{
			EgressId: utils.NewGuid(utils.EgressPrefix),
			Request: &rpc.StartEgressRequest_RoomComposite{
				RoomComposite: &livekit.RoomCompositeEgressRequest{
					RoomName: conf.RoomName,
					Layout:   "speaker-light",
					StreamOutputs: []*livekit.StreamOutput{{
						Protocol: livekit.StreamProtocol_RTMP,
						Urls:     []string{badStreamUrl, streamUrl1},
					}},
				},
			},
		}

		info, err := conf.client.StartEgress(context.Background(), "", req)
		require.NoError(t, err)
		require.Empty(t, info.Error)
		require.NotEmpty(t, info.EgressId)
		require.Equal(t, conf.RoomName, info.RoomName)
		require.Equal(t, livekit.EgressStatus_EGRESS_STARTING, info.Status)

		info = getUpdate(t, conf, info.EgressId)
		if info.Status == livekit.EgressStatus_EGRESS_STARTING {
			checkUpdate(t, conf, info.EgressId, livekit.EgressStatus_EGRESS_ACTIVE)
		} else {
			require.Equal(t, livekit.EgressStatus_EGRESS_ACTIVE, info.Status)
			checkUpdate(t, conf, info.EgressId, livekit.EgressStatus_EGRESS_ACTIVE)
		}

		info = stopEgress(t, conf, info.EgressId)
		require.Len(t, info.StreamResults, 2)
		for _, res := range info.StreamResults {
			if res.Url == redactedUrl1 {
				require.Equal(t, livekit.StreamInfo_FINISHED, res.Status)
				require.Empty(t, res.Error)
			} else {
				require.Equal(t, livekit.StreamInfo_FAILED, res.Status)
				require.NotEmpty(t, res.Error)
			}
		}
	})

	runWebTest(t, conf, "rtmp-failure", types.MimeTypeOpus, types.MimeTypeVP8, func(t *testing.T) {
		req := &rpc.StartEgressRequest{
			EgressId: utils.NewGuid(utils.EgressPrefix),
			Request: &rpc.StartEgressRequest_RoomComposite{
				RoomComposite: &livekit.RoomCompositeEgressRequest{
					RoomName: conf.RoomName,
					Layout:   "speaker-light",
					StreamOutputs: []*livekit.StreamOutput{{
						Protocol: livekit.StreamProtocol_RTMP,
						Urls:     []string{badStreamUrl},
					}},
				},
			},
		}

		info, err := conf.client.StartEgress(context.Background(), "", req)
		require.NoError(t, err)
		require.Empty(t, info.Error)
		require.NotEmpty(t, info.EgressId)
		require.Equal(t, conf.RoomName, info.RoomName)
		require.Equal(t, livekit.EgressStatus_EGRESS_STARTING, info.Status)

		// wait
		time.Sleep(time.Second * 5)

		info = getUpdate(t, conf, info.EgressId)
		if info.Status == livekit.EgressStatus_EGRESS_ACTIVE {
			checkUpdate(t, conf, info.EgressId, livekit.EgressStatus_EGRESS_FAILED)
		} else {
			require.Equal(t, livekit.EgressStatus_EGRESS_FAILED, info.Status)
		}
	})
}

func testRoomCompositeSegments(t *testing.T, conf *TestConfig) {
	for _, test := range []*testCase{
		{
			name: "rs-baseline",
			options: &livekit.EncodingOptions{
				AudioCodec:   livekit.AudioCodec_AAC,
				VideoCodec:   livekit.VideoCodec_H264_BASELINE,
				Width:        1920,
				Height:       1080,
				VideoBitrate: 4500,
			},
			filename:               "rs_{room_name}_{time}",
			playlist:               "rs_{room_name}_{time}.m3u8",
			filenameSuffix:         livekit.SegmentedFileSuffix_TIMESTAMP,
			expectVideoTranscoding: true,
		},
		{
			name: "rs-limit",
			options: &livekit.EncodingOptions{
				AudioCodec:   livekit.AudioCodec_AAC,
				VideoCodec:   livekit.VideoCodec_H264_BASELINE,
				Width:        1920,
				Height:       1080,
				VideoBitrate: 4500,
			},
			filename:               "rs_limit_{time}",
			playlist:               "rs_limit_{time}.m3u8",
			sessionTimeout:         time.Second * 20,
			expectVideoTranscoding: true,
		},
	} {
		runWebTest(t, conf, test.name, types.MimeTypeOpus, types.MimeTypeVP8, func(t *testing.T) {
			segmentOutput := &livekit.SegmentedFileOutput{
				FilenamePrefix: getFilePath(conf.ServiceConfig, test.filename),
				PlaylistName:   test.playlist,
				FilenameSuffix: test.filenameSuffix,
			}
			if test.filenameSuffix == livekit.SegmentedFileSuffix_INDEX && conf.GCPUpload != nil {
				segmentOutput.FilenamePrefix = test.filename
				segmentOutput.Output = &livekit.SegmentedFileOutput_Gcp{
					Gcp: conf.GCPUpload,
				}
			}

			room := &livekit.RoomCompositeEgressRequest{
				RoomName:       conf.RoomName,
				Layout:         "grid-dark",
				AudioOnly:      test.audioOnly,
				SegmentOutputs: []*livekit.SegmentedFileOutput{segmentOutput},
			}
			if test.options != nil {
				room.Options = &livekit.RoomCompositeEgressRequest_Advanced{
					Advanced: test.options,
				}
			}

			req := &rpc.StartEgressRequest{
				EgressId: utils.NewGuid(utils.EgressPrefix),
				Request: &rpc.StartEgressRequest_RoomComposite{
					RoomComposite: room,
				},
			}

			runSegmentsTest(t, conf, req, test)
		})
		if conf.Short {
			return
		}
	}
}

func testRoomCompositeMulti(t *testing.T, conf *TestConfig) {
	req := &rpc.StartEgressRequest{
		EgressId: utils.NewGuid(utils.EgressPrefix),
		Request: &rpc.StartEgressRequest_RoomComposite{
			RoomComposite: &livekit.RoomCompositeEgressRequest{
				RoomName: conf.room.Name(),
				Layout:   "grid-light",
				FileOutputs: []*livekit.EncodedFileOutput{{
					FileType: livekit.EncodedFileType_MP4,
					Filepath: getFilePath(conf.ServiceConfig, "rc_multiple_{time}"),
				}},
				StreamOutputs: []*livekit.StreamOutput{{
					Protocol: livekit.StreamProtocol_RTMP,
				}},
			},
		},
	}

	runMultipleTest(t, conf, req, true, true, false, livekit.SegmentedFileSuffix_TIMESTAMP)
}
