package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const UploadedAvatarURLPrefix = "/api/me/avatar?v="

// UserAvatar keeps image bytes out of the user rows read during authentication.
type UserAvatar struct {
	UserID string `gorm:"primaryKey;type:varchar(36)"`
	Data   []byte `gorm:"not null"`
	User   User   `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE"`
}

func (u *User) AvatarImageTag() string {
	address := strings.TrimSpace(u.AvatarURL)
	if address == "" {
		return ""
	}
	if tag, uploaded := strings.CutPrefix(address, UploadedAvatarURLPrefix); uploaded {
		return tag
	}
	sum := sha256.Sum256([]byte(address))
	return hex.EncodeToString(sum[:])
}
