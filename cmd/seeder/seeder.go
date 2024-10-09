// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

var (
	profilesCount int
	perUser       int
	threads       int
	relays        []string
	outputRelay   string
	seeder        = &cobra.Command{
		Use:   "seeder",
		Short: "seeder",
		Run: func(_ *cobra.Command, args []string) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := NewFetcher(ctx, relays, threads, profilesCount, perUser, outputRelay)
			f.StartFetching(ctx)
		},
	}
	initFlags = func() {
		seeder.Flags().IntVar(&profilesCount, "profilesCount", 1000000, "count of profiles to import")
		seeder.Flags().IntVar(&perUser, "perUser", 1000, "latest posts to import for each user")
		seeder.Flags().IntVar(&threads, "threads", profilesCount, "threads to import = profilesCount by default")
		seeder.Flags().StringArrayVar(&relays, "relays", make([]string, 0), "relays to fetch data from")
		seeder.Flags().StringVar(&outputRelay, "outputRelay", "", "relay to write data")
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
