package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
)

const (
	AvatarMaxBytes        = 5 << 20
	avatarSize            = 512
	avatarMaxSourcePixels = 16_000_000
)

var (
	ErrAvatarInvalid     = errors.New("请选择有效的 JPG、PNG 或 WebP 图片")
	ErrAvatarTooLarge    = errors.New("头像图片不能超过 5 MB")
	ErrAvatarPixelBudget = errors.New("头像图片不能超过 1600 万像素")
)

// SetAvatar replaces the user's image and address together. A nil file clears both.
func (p *ProfileService) SetAvatar(ctx context.Context, userID string, file []byte) (*model.User, error) {
	var imageData []byte
	var address string
	if file != nil {
		var err error
		imageData, err = encodeAvatar(file)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(imageData)
		address = model.UploadedAvatarURLPrefix + hex.EncodeToString(sum[:])
	}

	var user model.User
	err := p.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the owner first so replacement and removal are serialized.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		if imageData == nil {
			if err := tx.Delete(&model.UserAvatar{}, "user_id = ?", userID).Error; err != nil {
				return err
			}
		} else if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"data"}),
		}).Create(&model.UserAvatar{UserID: userID, Data: imageData}).Error; err != nil {
			return err
		}
		return tx.Model(&user).Update("avatar_url", address).Error
	})
	if err != nil {
		return nil, err
	}
	user.AvatarURL = address
	user.PopulateComputedFields()
	return &user, nil
}

func (p *ProfileService) UploadedAvatar(ctx context.Context, userID string) ([]byte, error) {
	var avatar model.UserAvatar
	err := p.repo.DB.WithContext(ctx).First(&avatar, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return avatar.Data, err
}

func encodeAvatar(data []byte) ([]byte, error) {
	if len(data) > AvatarMaxBytes {
		return nil, ErrAvatarTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrAvatarInvalid
	}
	if cfg.Width > avatarMaxSourcePixels/cfg.Height {
		return nil, ErrAvatarPixelBudget
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil || source.Bounds().Dx() != cfg.Width || source.Bounds().Dy() != cfg.Height {
		return nil, ErrAvatarInvalid
	}
	width, height := fitSize(cfg.Width, cfg.Height, avatarSize, avatarSize)
	avatar := image.NewRGBA(image.Rect(0, 0, width, height))
	resizeScaled(avatar, source, cfg.Width, cfg.Height)

	var encoded bytes.Buffer
	if format != "jpeg" && !isOpaqueImage(avatar) {
		err = png.Encode(&encoded, avatar)
	} else {
		err = jpeg.Encode(&encoded, avatar, &jpeg.Options{Quality: 88})
	}
	return encoded.Bytes(), err
}
