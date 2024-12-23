// SPDX-License-Identifier: ice License 1.0

package model

const (
	ModeratorRole Role = "moderator"
	AdminRole     Role = "admin"
	OwnerRole     Role = "owner"
	OthersRole    Role = ""

	KindCommunityJoin                  = 1750
	KindCommunityOwnershipTransferring = 1751
	KindCommunityBanUser               = 1752
	KindCommunityChangeDefinition      = 1753
	KindCommunityDefinition            = 31750
)
