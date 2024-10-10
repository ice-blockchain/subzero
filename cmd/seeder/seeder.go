// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"log"

	"github.com/davidbyttow/govips/v2/vips"
	"github.com/spf13/cobra"
)

var (
	profilesCount int
	perUser       int
	threads       int
	relays        []string
	outputRelay   string
	uploadKey     string
	seeder        = &cobra.Command{
		Use:   "seeder",
		Short: "seeder",
		Run: func(_ *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			defer func() {
				cancel()
				vips.Shutdown()
			}()
			vips.Startup(&vips.Config{})
			f := NewFetcher(ctx, relays, threads, profilesCount, perUser, outputRelay, uploadKey)
			f.StartFetching(ctx)
		},
	}
	initFlags = func() {
		seeder.Flags().IntVar(&profilesCount, "profilesCount", 1000000, "count of profiles to import")
		seeder.Flags().IntVar(&perUser, "perUser", 1000, "latest posts to import for each user")
		seeder.Flags().IntVar(&threads, "threads", profilesCount, "threads to import = profilesCount by default")
		seeder.Flags().StringArrayVar(&relays, "relays", make([]string, 0), "relays to fetch data from")
		seeder.Flags().StringVar(&outputRelay, "outputRelay", "", "relay to write data")
		seeder.Flags().StringVar(&uploadKey, "uploadKey", "", "key to upload webp generated photo if user is miss ont")
		if err := seeder.MarkFlagRequired("outputRelay"); err != nil {
			log.Fatal(err)
		}
		if err := seeder.MarkFlagRequired("relays"); err != nil {
			log.Fatal(err)
		}
	}
)

func init() {
	initFlags()
}

func main() {
	if err := seeder.Execute(); err != nil {
		log.Panic(err)
	}
}
