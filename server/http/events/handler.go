// SPDX-License-Identifier: ice License 1.0

package events

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"github.com/nbd-wtf/go-nostr"

	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

type (
	CommunityPostAuthor struct {
		Name              string `json:"name" example:"mahmutalijahad"`
		DisplayName       string `json:"displayName" example:"Mahmut Ali Jahad"`
		Avatar            string `json:"avatar" example:"https://example.com/something.webp"`
		Verified          bool   `json:"verified" example:"true"`
		masterKey         string `json:"-"`
		verifiedBadgePTag string `json:"-"`
	}
	PostMedia struct {
		Thumbnail *string `json:"thumbnail,omitempty" example:"https://example.com/image-preview.jpg"`
		URL       string  `json:"url" example:"https://example.com/image.jpg"`
		Type      string  `json:"type" example:"video"`
	}
	PostPreview struct {
		CreatedAt time.Time           `json:"createdAt" example:"2022-01-03T16:20:52.156534Z"`
		Media     []PostMedia         `json:"media,omitempty"`
		Author    CommunityPostAuthor `json:"author"`
		Type      string              `json:"type" example:"post"`
		Comments  int                 `json:"comments" example:"12"`
		Reposts   int                 `json:"reposts" example:"442"`
		Likes     int                 `json:"likes" example:"12000"`
		Content   string              `json:"content" example:"Something something https://example.com/someImage.webp https://example.com/someVideo.mp4 #online+"`
	}
)

func GetEventByAddress(ctx *gin.Context) {
	addressStr := ctx.Param("eventAddress")

	if addressStr == "" {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "event address is required"})
		return
	}

	it := query.GetStoredEvents(ctx, model.Filter{
		Addresses: []string{addressStr},
		Limit:     1,
	})

	var event *model.Event
	for ev, err := range it {
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		event = ev
		break
	}

	if event == nil {
		ctx.AbortWithStatus(http.StatusNotFound)
		return
	}

	ctx.JSON(http.StatusOK, event)
}

func GetEventPreview(ctx *gin.Context) {
	addressStr := ctx.Param("eventAddress")
	if addressStr == "" {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "event address is required"})
		return
	}
	var eventPreview PostPreview
	it := query.GetStoredEvents(ctx, eventPreviewFilters(addressStr))
	events := 0
	for ev, err := range it {
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		err = updatePreviewWithEvent(&eventPreview, ev)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		events += 1
	}
	if events == 0 {
		ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}
	ctx.JSON(http.StatusOK, eventPreview)
}

func updatePreviewWithEvent(preview *PostPreview, ev *model.Event) (err error) {
	switch ev.Kind {
	case nostr.KindProfileMetadata:
		kind0Data, err := extractProfileContentMetadata(ev.Content)
		if err != nil {
			return errors.Wrapf(err, "failed to parse author's metadata: %v", ev.Content)
		}
		if !preview.Author.Verified && preview.Author.verifiedBadgePTag == ev.GetMasterPublicKey() {
			preview.Author.Verified = true
		}
		preview.Author.Name = kind0Data.Name
		preview.Author.DisplayName = kind0Data.DisplayName
		preview.Author.Avatar = kind0Data.Picture
		preview.Author.masterKey = ev.GetMasterPublicKey()

	case nostr.KindBadgeAward:
		aTags := ev.Tags.GetAll([]string{"a"})
		for _, aTag := range aTags {
			if len(aTag) >= 2 {
				eventAuthor := preview.Author.masterKey
				if aTag[1] == fmt.Sprintf("%d:%s:verified", nostr.KindBadgeDefinition, ev.PubKey) {
					if pTag := ev.Tags.GetFirst([]string{"p"}); eventAuthor != "" && pTag != nil && pTag.Value() == eventAuthor {
						preview.Author.Verified = true
						break
					} else if eventAuthor == "" {
						preview.Author.verifiedBadgePTag = pTag.Value()
					}
				}
			}
		}
	case nostr.KindTextNote, model.CustomIONKindEditableTextNote, nostr.KindArticle:
		preview.CreatedAt = ev.CreatedAt.Time()
		if ev.Content != "" {
			preview.Content, err = model.ReplacePMO(ev, func(orig, replace string) (bool, string) {
				needReplace := strings.HasPrefix(orig, "ion:nprofile") || strings.HasPrefix(orig, "nostr:npub")
				if !needReplace {
					return false, ""
				}
				if strings.HasPrefix(replace, "[@") && strings.Contains(replace, "]") {
					return true, replace[1:strings.Index(replace, "]")]
				}
				return true, replace
			})
			if err != nil {
				return errors.Wrap(err, "failed to replace mentions using PMO tags")
			}
		} else if richTextContent := model.ExtractRichTextContent(ev); richTextContent != "" {
			preview.Content = richTextContent
		}
		if preview.Author.masterKey == "" {
			preview.Author.masterKey = ev.GetMasterPublicKey()
			if !preview.Author.Verified && preview.Author.verifiedBadgePTag == ev.GetMasterPublicKey() {
				preview.Author.Verified = true
			}
		}
		switch ev.Kind {
		case nostr.KindArticle:
			preview.Type = "article"
		default:
			if ev.HasVideoIMeta() {
				preview.Type = "video"
			} else {
				preview.Type = "post"
			}
		}
		for _, imeta := range ev.Tags.GetAll([]string{"imeta"}) {
			media, err := model.ParseIMeta(imeta)
			if err != nil {
				return errors.Wrapf(err, "failed to parse media tag %v", imeta)
			}
			preview.Media = append(preview.Media, convertMediaToPreview(media))
		}
	case model.KindJobNostrEventCount + 1000:
		requestPayload := ev.Tags.GetFirst([]string{"request"})
		var dvmReq model.Event
		if err = json.Unmarshal([]byte(requestPayload.Value()), &dvmReq); err != nil {
			return errors.Wrapf(err, "failed to unmarshal dvm request %v", requestPayload)
		}
		if dvmReq.Kind == model.KindJobNostrEventCount {
			var reqKinds []struct {
				Kinds []int `json:"kinds"`
			}
			if err = json.Unmarshal([]byte(dvmReq.Content), &reqKinds); err != nil {
				return errors.Wrapf(err, "failed to unmarshal dvm payload %v", dvmReq.Content)
			}
			if len(reqKinds) == 0 || len(reqKinds[0].Kinds) == 0 {
				return errors.New("no kinds specified in dvm request")
			}
			kind := reqKinds[0].Kinds[0]
			counter, err := strconv.Atoi(ev.Content)
			if err != nil {
				var reactions map[string]int
				if err = json.Unmarshal([]byte(ev.Content), &reactions); err != nil {
					return errors.Wrapf(err, "failed to unmarshal counter %v", ev.Content)
				}
				counter = reactions["+"]
			}
			switch kind {
			case nostr.KindGenericRepost, nostr.KindRepost:
				preview.Reposts = counter
			case model.CustomIONKindEditableTextNote, nostr.KindTextNote:
				preview.Comments = counter
			case nostr.KindReaction:
				preview.Likes = counter
			}
		}
	}
	return nil
}

func convertMediaToPreview(media map[string]string) PostMedia {
	var thumbnail *string
	if thumb, ok := media["thumb"]; ok {
		thumbnail = &thumb
	}
	var mediaType string
	switch m, ok := media["m"]; {
	case ok && strings.HasPrefix(m, "image/"):
		mediaType = "image"
	case ok && strings.HasPrefix(m, "video/"):
		mediaType = "video"
	default:
		if strings.HasSuffix(media["url"], "mp4") {
			mediaType = "video"
		} else {
			mediaType = "image"
		}
	}
	return PostMedia{
		Thumbnail: thumbnail,
		URL:       media["url"],
		Type:      mediaType,
	}
}

func extractProfileContentMetadata(contentJSON string) (*model.ProfileMetadataContent, error) {
	var content model.ProfileMetadataContent

	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal profile metadata content")
	}
	return &content, nil
}

func eventPreviewFilters(eventAddress string) model.Filter {
	return model.Filter{
		Kinds: []int{
			model.CustomIONKindEditableTextNote,
			nostr.KindTextNote,
			nostr.KindRepost,
			model.CustomIONKindRepostOfEditableTextNote,
			model.CustomIONKindRepostOfArticle,
			nostr.KindArticle,
		},
		Addresses: []string{eventAddress},
		Limit:     1,
		Search:    "include:dependencies:kind30175>kind6400+kind30175+group+reply include:dependencies:kind30175>kind6400+kind16+group+e include:dependencies:kind30175>kind6400+kind30175+group+q include:dependencies:kind30175>kind6400+kind7+group+content include:dependencies:kind30175>kind6400+kind1754+group+content include:dependencies:kind30175>kind0 include:dependencies:kind30175>kind30008+profile_badges>kind30009>kind8 include:dependencies:kind30175>kind10000 include:dependencies:kind1>kind6400+kind30175+group+reply include:dependencies:kind1>kind6400+kind16+group+e include:dependencies:kind1>kind6400+kind30175+group+q include:dependencies:kind1>kind6400+kind7+group+content include:dependencies:kind1>kind6400+kind1754+group+content include:dependencies:kind1>kind0 include:dependencies:kind1>kind30008+profile_badges>kind30009>kind8 include:dependencies:kind1>kind10000 include:dependencies:kind30023>kind6400+kind30175+group+reply include:dependencies:kind30023>kind6400+kind16+group+e include:dependencies:kind30023>kind6400+kind30175+group+q include:dependencies:kind30023>kind6400+kind7+group+content include:dependencies:kind30023>kind6400+kind1754+group+content include:dependencies:kind30023>kind0 include:dependencies:kind30023>kind30008+profile_badges>kind30009>kind8 include:dependencies:kind30023>kind10000 include:dependencies:kind16>kind6400+kind30175+group+reply include:dependencies:kind16>kind6400+kind16+group+e include:dependencies:kind16>kind6400+kind30175+group+q include:dependencies:kind16>kind6400+kind7+group+content include:dependencies:kind16>kind6400+kind1754+group+content references:false expiration:false !amarker:reply !emarker:reply include:dependencies:kind30175>kind31175 include:dependencies:kind1>kind31175 include:dependencies:kind30023>kind31175",
	}
}
