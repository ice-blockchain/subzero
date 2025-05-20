// SPDX-License-Identifier: ice License 1.0

package query

import (
	"strconv"

	"github.com/bzick/tokenizer"
	"github.com/cockroachdb/errors"
)

type (
	token = tokenizer.TokenKey

	filterDependencyStart struct {
		Kind          int
		ProfileBadges bool
		Tag           string
	}

	filterDependencyReduce struct {
		Kinds   []int
		Author  string
		Group   bool
		Tag     string
		Context string
	}

	filterDependency struct {
		Start  filterDependencyStart
		Reduce filterDependencyReduce
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

	tokenLiteralTagQ

	tokenLiteralGroupTagDetailP
	tokenLiteralDetailTagE
	tokenLiteralDetailTagP

	tokenLiteralPipe
)

var (
	errDepParserUnexpectedToken = errors.New("unexpected token")
	errDepParserInvalidToken    = errors.New("invalid token")

	parserTokens = map[token][]string{
		tokenSearchExpr:  {">"},
		tokenCondDetail:  {"+"},
		tokenCondInclude: {"@"},

		tokenLiteralKind:          {"kind"},
		tokenLiteralGroup:         {"group"},
		tokenLiteralContent:       {"content"},
		tokenLiteralProfileBadges: {"profile_badges"},
		tokenLiteralReply:         {"reply", "root"},

		tokenLiteralDetailTagE: {"+e"},
		tokenLiteralDetailTagP: {"+p+"},
		tokenLiteralTagQ:       {"q"},

		tokenLiteralGroupTagDetailP: {"group+p"},
	}

	parserKnownSequences = []filterSequence{
		// kindXXX>kind30008+profile_badges>kind30009>kind8.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralProfileBadges,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenizer.TokenUndef,
			},
		},
		// kind30008+profile_badges>kind30009>kind8.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralProfileBadges,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenizer.TokenUndef,
			},
		},
		// kind1+q>kind10002.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralTagQ,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenizer.TokenUndef,
			},
		},
		// kind1>$logged_in_user_pubkey@kind1+e+root/reply.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenizer.TokenKeyword,
				tokenCondInclude,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenLiteralDetailTagE,
				tokenCondDetail,
				tokenLiteralReply,
				tokenizer.TokenUndef,
			},
		},
		// kind1>$logged_in_user_pubkey@kind1+q.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenizer.TokenKeyword,
				tokenCondInclude,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralTagQ,
				tokenizer.TokenUndef,
			},
		},
		// kind1>$logged_in_user_pubkey@kind6 / kind1>$logged_in_user_pubkey@kind7.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenizer.TokenKeyword,
				tokenCondInclude,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind1+group+root/reply.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralReply,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind6+group+e.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenLiteralDetailTagE,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind1+group+q.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralTagQ,
				tokenizer.TokenUndef,
			},
		},
		// kind0>kind6400+kind3+group+p.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralGroupTagDetailP,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind6400+kind7+group+content.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralContent,
				tokenizer.TokenUndef,
			},
		},
		// kind1>kind0 / kind6>kind10002 / kind3>kind0.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenizer.TokenUndef,
			},
		},
		// kind1/30023>kind6400+kind1754+group+content.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenCondDetail,
				tokenLiteralGroup,
				tokenCondDetail,
				tokenLiteralContent,
				tokenizer.TokenUndef,
			},
		},
		// kind3>kind0+p+|key1,key2,keyN|.
		{
			Tokens: []token{
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenSearchExpr,
				tokenLiteralKind, tokenizer.TokenKeyword,
				tokenLiteralDetailTagP,
				tokenizer.TokenString,
				tokenizer.TokenUndef,
			},
		},
	}

	dependenciesParser *tokenizer.Tokenizer
)

func init() {
	dependenciesParser = tokenizer.New()
	dependenciesParser.SetWhiteSpaces([]byte{' ', '\t'})
	dependenciesParser.AllowKeywordSymbols(tokenizer.Numbers, tokenizer.Numbers)
	dependenciesParser.DefineStringToken(tokenLiteralPipe, `|`, `|`).SetEscapeSymbol(tokenizer.BackSlash)
	for k, v := range parserTokens {
		dependenciesParser.DefineTokens(k, v)
	}
}

func (s *filterSequence) Parse(stream *tokenizer.Stream) (*filterDependency, error) {
	var filter filterDependency

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

		case tokenLiteralDetailTagE, tokenLiteralTagQ:
			val := stream.CurrentToken().ValueString()
			if val[0] == '+' {
				val = val[1:]
			}
			if start {
				filter.Start.Tag = val
			} else {
				filter.Reduce.Tag = val
			}
		case tokenLiteralGroupTagDetailP:
			filter.Reduce.Tag = "p"
			filter.Reduce.Group = true

		case tokenLiteralGroup:
			filter.Reduce.Group = true

		case tokenSearchExpr:
			start = false

		case tokenizer.TokenString:
			filter.Reduce.Author = stream.CurrentToken().ValueUnescapedString()

		case tokenizer.TokenKeyword:
			if stream.PrevToken().Is(tokenLiteralKind) {
				break
			}
			filter.Reduce.Author = stream.CurrentToken().ValueString()

		case tokenLiteralProfileBadges:
			filter.Start.ProfileBadges = true

		case tokenLiteralKind:
			str := stream.NextToken().ValueString()
			val, err := strconv.ParseInt(str, 10, 64)
			if err != nil {
				return nil, errors.Wrapf(errDepParserInvalidToken, "failed to parse kind value %q: %v", str, err)
			}
			if start {
				filter.Start.Kind = int(val)
			} else {
				filter.Reduce.Kinds = append(filter.Reduce.Kinds, int(val))
			}
		}

		stream.GoNext()
	}

	return &filter, nil
}

func parseDepRequest(in string) (*filterDependency, error) {
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
