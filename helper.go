// Copyright © 2019 Martin Tournoij – This file is part of GoatCounter and
// published under the terms of a slightly modified EUPL v1.2 license, which can
// be found in the LICENSE file or at https://license.goatcounter.com

//go:generate go run gen.go

package goatcounter

import (
	"context"
	"fmt"
	"time"

	"zgo.at/zdb"
	"zgo.at/zhttp/ctxkey"
	"zgo.at/zhttp/ztpl"
	"zgo.at/zstd/zcrypto"
)

// State column values.
const (
	StateActive  = "a"
	StateRequest = "r"
	StateDeleted = "d"
)

var States = []string{StateActive, StateRequest, StateDeleted}

// Now gets the current time in UTC; can be overwritten in tests.
var Now = func() time.Time { return time.Now().UTC() }

// WithSite adds the site to the context.
func WithSite(ctx context.Context, s *Site) context.Context {
	return context.WithValue(ctx, ctxkey.Site, s)
}

// GetSite gets the current site.
func GetSite(ctx context.Context) *Site {
	s, _ := ctx.Value(ctxkey.Site).(*Site)
	return s
}

// MustGetSite behaves as GetSite(), panicking if this fails.
func MustGetSite(ctx context.Context) *Site {
	s, ok := ctx.Value(ctxkey.Site).(*Site)
	if !ok {
		panic("MustGetSite: no site on context")
	}
	return s
}

// WithUser adds the site to the context.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, ctxkey.User, u)
}

// GetUser gets the currently logged in user.
func GetUser(ctx context.Context) *User {
	u, _ := ctx.Value(ctxkey.User).(*User)
	return u
}

// NewContext creates a new context with the all the request values set.
//
// Useful for tests, or for "removing" the timeout on the request context so it
// can be passed to background functions.
func NewContext(ctx context.Context) context.Context {
	n := zdb.WithDB(context.Background(), zdb.MustGetDB(ctx))
	n = context.WithValue(n, ctxkey.User, GetUser(ctx))
	n = context.WithValue(n, ctxkey.Site, GetSite(ctx))
	return n
}

func EmailTemplate(tplname string, args interface{}) func() ([]byte, error) {
	return func() ([]byte, error) {
		return ztpl.ExecuteBytes(tplname, args)
	}
}

func Reset() {
	sitesCache.Flush()
	sitesCacheHostname.Flush()
	cachePaths.Flush()
	cacheUA.Flush()
	cacheBrowsers.Flush()
	cacheSystems.Flush()
	changedTitles.Flush()
	locationsCache.Flush()
}

// TODO: Move to zdb
func interval(ctx context.Context, days int) string {
	if zdb.PgSQL(ctx) {
		return fmt.Sprintf(" now() - interval '%d days' ", days)
	}
	return fmt.Sprintf(" datetime(datetime(), '-%d days') ", days)
}

const numChars = 12

// Compress all the data in to 12 chunks.
func ChunkStat(stats []Stat) (int, []int) {
	var (
		chunked   = make([]int, numChars)
		chunkSize = len(stats) * 24 / numChars
		max       = 0
		chunk     = 0
		i         = 0
		n         = 0
	)
	for _, stat := range stats {
		for _, h := range stat.HourlyUnique {
			i++
			chunk += h
			if i == chunkSize {
				chunked[n] = chunk
				if chunk > max {
					max = chunk
				}
				n++
				chunk, i = 0, 0
			}
		}
	}

	return max, chunked
}

func NewBufferKey(ctx context.Context) (string, error) {
	secret := zcrypto.Secret256()
	err := zdb.TX(ctx, func(ctx context.Context) error {
		err := zdb.Exec(ctx, `delete from store where key='buffer-secret'`, nil)
		if err != nil {
			return err
		}

		err = zdb.Exec(ctx, `insert into store (key, value) values ('buffer-secret', :s)`, zdb.A{"s": secret})
		return err
	})
	if err != nil {
		return "", fmt.Errorf("NewBufferKey: %w", err)
	}
	return secret, nil
}

func LoadBufferKey(ctx context.Context) ([]byte, error) {
	var key []byte
	err := zdb.Get(ctx, &key, `select value from store where key='buffer-secret'`)
	if err != nil {
		return nil, fmt.Errorf("LoadBufferKey: %w", err)
	}
	return key, nil
}
