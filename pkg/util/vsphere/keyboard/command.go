// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package keyboard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TokenKind identifies the kind of a parsed boot command Token.
type TokenKind int

const (
	// TokenLiteral is a single printable ASCII character to type.
	TokenLiteral TokenKind = iota

	// TokenSpecialKey is a named special key, e.g. "esc" or "f1".
	TokenSpecialKey

	// TokenWait pauses for Wait before processing the next token.
	TokenWait

	// TokenModifierOn holds down the named modifier key for every
	// subsequent token until a matching TokenModifierOff.
	TokenModifierOn

	// TokenModifierOff releases the named modifier key held by a prior
	// TokenModifierOn.
	TokenModifierOff
)

// Token is a single, parsed unit of a boot command string.
type Token struct {
	Kind TokenKind

	// Literal is set when Kind is TokenLiteral.
	Literal rune

	// Special is the lowercase special key name, set when Kind is
	// TokenSpecialKey.
	Special string

	// Wait is set when Kind is TokenWait.
	Wait time.Duration

	// Modifier is the lowercase modifier key name, e.g. "leftshift", set
	// when Kind is TokenModifierOn or TokenModifierOff.
	Modifier string
}

// tagPattern matches a single <...> token, e.g. <esc>, <wait3m30s>,
// <leftShiftOn>.
var tagPattern = regexp.MustCompile(`<[^<>]+>`)

// waitBareDigitsPattern matches a wait token whose duration is expressed as
// a bare number of seconds, e.g. wait5, wait10 (wait with no suffix at all
// means 1 second and is handled separately).
var waitBareDigitsPattern = regexp.MustCompile(`^wait(\d+)$`)

// ParseCommands renders each command string with render (if non-nil), then
// tokenizes every rendered command in order into a single flat slice of
// Tokens.
func ParseCommands(commands []string, render func(string) string) ([]Token, error) {
	var tokens []Token

	for i, cmd := range commands {
		rendered := cmd
		if render != nil {
			rendered = render(cmd)
		}

		cmdTokens, err := parseCommand(rendered)
		if err != nil {
			return nil, fmt.Errorf("failed to parse commands[%d]: %w", i, err)
		}
		tokens = append(tokens, cmdTokens...)
	}

	return tokens, nil
}

// TotalWait returns the sum of every TokenWait token's Wait duration.
func TotalWait(tokens []Token) time.Duration {
	var total time.Duration
	for _, t := range tokens {
		if t.Kind == TokenWait {
			total += t.Wait
		}
	}
	return total
}

// parseCommand tokenizes a single, already-rendered command string.
func parseCommand(s string) ([]Token, error) {
	var tokens []Token

	for len(s) > 0 {
		loc := tagPattern.FindStringIndex(s)
		if loc == nil {
			tokens = append(tokens, literalTokens(s)...)
			break
		}

		tokens = append(tokens, literalTokens(s[:loc[0]])...)

		tag := s[loc[0]+1 : loc[1]-1]
		tok, err := parseTag(tag)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)

		s = s[loc[1]:]
	}

	return tokens, nil
}

// literalTokens converts each rune in s into its own TokenLiteral.
func literalTokens(s string) []Token {
	tokens := make([]Token, 0, len(s))
	for _, r := range s {
		tokens = append(tokens, Token{Kind: TokenLiteral, Literal: r})
	}
	return tokens
}

// parseTag parses the contents of a single <...> token (without the angle
// brackets).
func parseTag(tag string) (Token, error) {
	lower := strings.ToLower(tag)

	switch {
	case lower == "wait":
		return Token{Kind: TokenWait, Wait: time.Second}, nil

	case strings.HasPrefix(lower, "wait"):
		return parseWaitTag(tag, lower)

	case strings.HasSuffix(lower, "on") && isModifierName(strings.TrimSuffix(lower, "on")):
		return Token{Kind: TokenModifierOn, Modifier: strings.TrimSuffix(lower, "on")}, nil

	case strings.HasSuffix(lower, "off") && isModifierName(strings.TrimSuffix(lower, "off")):
		return Token{Kind: TokenModifierOff, Modifier: strings.TrimSuffix(lower, "off")}, nil

	default:
		if _, ok := specialKeys[lower]; ok {
			return Token{Kind: TokenSpecialKey, Special: lower}, nil
		}
		return Token{}, fmt.Errorf("unknown boot command token <%s>", tag)
	}
}

// parseWaitTag parses the suffix of a wait token, e.g. "wait5" (bare
// seconds) or "wait3m30s" (a Go duration string).
func parseWaitTag(tag, lower string) (Token, error) {
	if m := waitBareDigitsPattern.FindStringSubmatch(lower); m != nil {
		secs, err := strconv.Atoi(m[1])
		if err != nil {
			return Token{}, fmt.Errorf("invalid wait token <%s>: %w", tag, err)
		}
		return Token{Kind: TokenWait, Wait: time.Duration(secs) * time.Second}, nil
	}

	d, err := time.ParseDuration(lower[len("wait"):])
	if err != nil {
		return Token{}, fmt.Errorf("invalid wait token <%s>: %w", tag, err)
	}

	return Token{Kind: TokenWait, Wait: d}, nil
}
