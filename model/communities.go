// SPDX-License-Identifier: ice License 1.0

package model

const (
	ModeratorRole Role = "moderator"
	AdminRole     Role = "admin"
	OwnerRole     Role = "owner"
	OthersRole    Role = ""

	CustomIONKindCommunityJoin                  = 1750
	CustomIONKindCommunityOwnershipTransferring = 1751
	CustomIONKindCommunityBanUser               = 1752
	CustomIONKindCommunityChangeDefinition      = 1753
	CustomIONKindCommunityDefinition            = 31750
)

func GetCommunityRoleByPubkey(pubkey string, communityDefinitionEvent *Event) Role {
	if communityDefinitionEvent.PubKey == pubkey {
		return OwnerRole
	}
	pTags := communityDefinitionEvent.Tags.GetAll([]string{"p"})
	if pTags == nil {
		return OthersRole
	}
	for _, pTag := range pTags {
		if pTag.Key() == "p" && len(pTag) > 3 && pTag.Value() == pubkey {
			return Role(pTag[3])
		}
	}

	return OthersRole
}
