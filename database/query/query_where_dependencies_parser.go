// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"

	"github.com/bzick/tokenizer"
	"github.com/cockroachdb/errors"
)

type (
	token = tokenizer.TokenKey

	filterDependenciesStart struct {
		Kind          int
		ProfileBadges bool
		Tag           string
	}

	filterDependenciesReduce struct {
		Kinds   []int
		Author  string
		Group   bool
		Tag     string
		Context string
	}

	filterDependencies struct {
		Start  filterDependenciesStart
		Reduce filterDependenciesReduce
	}

	filterSequence struct {
		Tokens []token
	}
)

const (
	tokenSearchExpr token = iota + 1
	tokenCondDetail
	tokenCondInclude

	tokenLiteralKind
	tokenLiteralProfileBadges
	tokenLiteralGroup
	tokenLiteralContent
	tokenLiteralReply

	tokenLiteralTagE
	tokenLiteralTagQ
)

var (
	errDepParserUnexpectedToken = errors.New("unexpected token")

	parserTokens = map[token][]string{
		tokenSearchExpr:  {">"},
		tokenCondDetail:  {"+"},
		tokenCondInclude: {"@"},

		tokenLiteralKind:          {"kind"},
		tokenLiteralGroup:         {"group"},
		tokenLiteralContent:       {"content"},
		tokenLiteralProfileBadges: {"profile_badges"},
		tokenLiteralReply:         {"reply", "root"},

		tokenLiteralTagE: {"e"},
		tokenLiteralTagQ: {"q"},
	}

	parserKnownSequences = []filterSequence{
		// kind30008+profile_badges>kind30009>kind8.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralProfileBadges,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenizer.TokenUndef,
			},
		},
		// kind1+q>kind10002.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralTagQ,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenizer.TokenUndef,
			},
		},
		// kind1>$logged_in_user_pubkey@kind1+e+root.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenizer.TokenKeyword,
				tokenCondInclude,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralTagE,
				tokenCondDetail,
				tokenLiteralReply,
				tokenizer.TokenUndef,
			},
		},
		// kind1>$logged_in_user_pubkey@kind1+q.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenizer.TokenKeyword,
				tokenCondInclude,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralTagQ,
				tokenizer.TokenUndef,
			},
		},
		// kind1>$logged_in_user_pubkey@kind6 / kind1>$logged_in_user_pubkey@kind7.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenizer.TokenKeyword,
				tokenCondInclude,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind1+group+root/reply.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralReply,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind1+group+e.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralTagE,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind1+group+q.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralTagQ,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind1+group+content.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralContent,
				tokenizer.TokenUndef,
			},
		},
		// kindN>kindN.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenInteger,
				tokenizer.TokenUndef,
			},
		},
	}

	dependenciesParser *tokenizer.Tokenizer
)

func init() {
	dependenciesParser = tokenizer.New()
	dependenciesParser.SetWhiteSpaces([]byte{' ', '\t'})
	dependenciesParser.
		AllowNumberUnderscore().
		AllowKeywordUnderscore().
		AllowNumbersInKeyword()
	for k, v := range parserTokens {
		dependenciesParser.DefineTokens(k, v)
	}
}

func (s *filterSequence) Parse(stream *tokenizer.Stream) (*filterDependencies, error) {
	var filter filterDependencies

	if s == nil {
		return nil, errors.Wrap(errDepParserUnexpectedToken, "sequence not found")
	}

	tokens := s.Tokens
	if tokens[len(tokens)-1] == tokenizer.TokenUndef {
		tokens = tokens[:len(tokens)-1]
	}

	start := true
	for _, token := range tokens {
		if !stream.CurrentToken().Is(token) || !stream.IsValid() {
			panic("unexpected token in the parsed stream: " + strconv.Itoa(int(token)))
		}

		switch token {
		case tokenLiteralContent, tokenLiteralReply:
			filter.Reduce.Context = stream.CurrentToken().ValueString()

		case tokenLiteralTagE, tokenLiteralTagQ:
			if start {
				filter.Start.Tag = stream.CurrentToken().ValueString()
			} else {
				filter.Reduce.Tag = stream.CurrentToken().ValueString()
			}

		case tokenLiteralGroup:
			filter.Reduce.Group = true

		case tokenSearchExpr:
			start = false

		case tokenizer.TokenKeyword:
			filter.Reduce.Author = stream.CurrentToken().ValueString()

		case tokenLiteralProfileBadges:
			filter.Start.ProfileBadges = true

		case tokenLiteralKind:
			val := int(stream.NextToken().ValueInt64())
			if start {
				filter.Start.Kind = val
			} else {
				filter.Reduce.Kinds = append(filter.Reduce.Kinds, val)
			}
		}

		stream.GoNext()
	}

	return &filter, nil
}

func parseDepRequest(in string) (*filterDependencies, error) {
	stream := dependenciesParser.ParseString(in)
	defer stream.Close()

	if !stream.IsValid() {
		return nil, errors.Wrap(errDepParserUnexpectedToken, "stream is not valid")
	}

	var currentSequence *filterSequence
	for i := range parserKnownSequences {
		seq := &parserKnownSequences[i]
		if !(stream.CurrentToken().Is(seq.Tokens[0]) && stream.IsNextSequence(seq.Tokens[1:]...)) {
			continue
		}

		currentSequence = seq
		break
	}

	f, err := currentSequence.Parse(stream)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse filter expression %q", in)
	}

	return f, nil
}
