package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// CanvasExcludedGroup 是账号接入合同明确排除的原始分组标识。
	CanvasExcludedGroup = "神秘分组"

	// CanvasGrantStatusActive 表示授权可用于账号同步和管理 Key 操作。
	CanvasGrantStatusActive = "active"
	// CanvasGrantStatusRevoked 表示授权及其派生管理 Key 已撤销。
	CanvasGrantStatusRevoked = "revoked"

	// CanvasManagedTokenStatusActive 表示管理关系仍绑定原始 Token。
	CanvasManagedTokenStatusActive = "active"
	// CanvasManagedTokenStatusRevoked 表示管理关系已随授权撤销。
	CanvasManagedTokenStatusRevoked = "revoked"
	// CanvasManagedTokenStatusChanged 表示 Token 已被人工修改，后续同步或重新授权不得自动恢复。
	CanvasManagedTokenStatusChanged = "changed"
)

var canvasOperationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

var (
	// ErrCanvasGrantInvalid 表示授权不存在、已过期、已撤销或不再属于可用用户。
	ErrCanvasGrantInvalid = errors.New("canvas grant is invalid")
	// ErrCanvasGroupExcluded 表示请求命中了合同明确排除的分组。
	ErrCanvasGroupExcluded = errors.New("canvas group is excluded")
	// ErrCanvasGroupUnavailable 表示用户当前无权使用该分组。
	ErrCanvasGroupUnavailable = errors.New("canvas group is unavailable")
	// ErrCanvasAutoGroupEmpty 表示 Auto 排除和权限过滤后没有可用实际分组。
	ErrCanvasAutoGroupEmpty = errors.New("canvas auto group scope is empty")
	// ErrCanvasIdempotencyConflict 表示相同 operation_id 被用于不同请求。
	ErrCanvasIdempotencyConflict = errors.New("canvas idempotency operation conflicts with the original request")
	// ErrCanvasManagedTokenChanged 表示原 Token 被禁用或修改了不可自动恢复的关键权限。
	ErrCanvasManagedTokenChanged = errors.New("canvas managed token changed")
	// ErrCanvasManagedTokenMissing 表示管理关系仍在，但原 Token 行已删除。
	ErrCanvasManagedTokenMissing = errors.New("canvas managed token missing")
	// ErrCanvasManagedTokenGroupMismatch 表示原 Token 仍存在但分组已被人工改动。
	ErrCanvasManagedTokenGroupMismatch = errors.New("canvas managed token group mismatch")
	// ErrCanvasTokenLimitReached 表示用户 Token 数量已达到站点上限。
	ErrCanvasTokenLimitReached = errors.New("canvas managed token limit reached")
	// ErrCanvasPermissionRevisionConflict 表示受理请求携带的权限修订已过期。
	ErrCanvasPermissionRevisionConflict = errors.New("canvas permission revision conflict")
	// ErrCanvasAcceptanceMismatch 表示受理请求的 grant、实例、Token 或分组身份不匹配。
	ErrCanvasAcceptanceMismatch = errors.New("canvas acceptance identity mismatch")
)

// CanvasGrant 保存一个 Canvas 实例对某个用户的受限授权。
// TokenHash 是原始 Bearer token 的 HMAC；原始 token 只在兑换成功时返回一次。
type CanvasGrant struct {
	ID         string `json:"id" gorm:"type:varchar(36);primaryKey"`
	ClientID   string `json:"client_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_canvas_grant_identity,priority:1"`
	InstanceID string `json:"instance_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_canvas_grant_identity,priority:2"`
	UserID     int    `json:"user_id" gorm:"not null;index;uniqueIndex:idx_canvas_grant_identity,priority:3"`
	TokenHash  string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	Scopes     string `json:"scopes" gorm:"type:varchar(255);not null"`
	Status     string `json:"status" gorm:"type:varchar(16);not null;index"`
	Revision   int64  `json:"revision" gorm:"type:bigint;not null;default:1"`
	IssuedAt   int64  `json:"issued_at" gorm:"type:bigint;not null"`
	ExpiresAt  int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	RevokedAt  int64  `json:"revoked_at" gorm:"type:bigint;not null"`
	CreatedAt  int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt  int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

// CanvasManagedToken 保存 grant、原始分组和实际 Token 的服务端归属关系。
// PermissionRevision 只跟随授权元数据变化，不跟随余额扣减或价格变化。
type CanvasManagedToken struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	GrantID            string `json:"grant_id" gorm:"type:varchar(36);not null;index;uniqueIndex:idx_canvas_managed_group,priority:1"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	GroupID            string `json:"group_id" gorm:"type:varchar(191);not null;uniqueIndex:idx_canvas_managed_group,priority:2"`
	TokenID            int    `json:"token_id" gorm:"not null;uniqueIndex"`
	CredentialRevision int64  `json:"credential_revision" gorm:"type:bigint;not null"`
	// KeyFingerprint 在受控轮换后固定新 Key；旧版空值仍由 Canvas 的已存指纹校验。
	KeyFingerprint        string `json:"-" gorm:"type:varchar(64);not null;default:''"`
	PermissionRevision    int64  `json:"permission_revision" gorm:"type:bigint;not null"`
	PermissionFingerprint string `json:"-" gorm:"type:char(64);not null"`
	AutoGroups            string `json:"-" gorm:"type:text"`
	Status                string `json:"status" gorm:"type:varchar(16);not null;index"`
	ReplacedByID          *int64 `json:"replaced_by_id,omitempty" gorm:"index"`
	CreatedAt             int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt             int64  `json:"updated_at" gorm:"type:bigint;not null"`
	RevokedAt             int64  `json:"revoked_at" gorm:"type:bigint;not null"`
}

// CanvasIdempotencyOperation 将 operation_id 和请求摘要绑定到唯一管理关系。
// 相同 operation_id 的网络重试返回原结果；不同摘要永远返回冲突。
type CanvasIdempotencyOperation struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	GrantID        string `json:"grant_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_canvas_operation,priority:1"`
	OperationID    string `json:"operation_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_canvas_operation,priority:2"`
	RequestDigest  string `json:"request_digest" gorm:"type:char(64);not null"`
	ManagedTokenID int64  `json:"managed_token_id" gorm:"not null;index"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;not null"`
}

// CanvasManagedAuthority 汇总一次权威校验后的 grant、用户、管理关系与 Token。
type CanvasManagedAuthority struct {
	Grant   CanvasGrant
	User    User
	Managed CanvasManagedToken
	Token   Token
}

// CanvasManagedTokenInput 描述幂等创建或恢复管理 Token 所需的服务端字段。
type CanvasManagedTokenInput struct {
	GrantID     string
	GroupID     string
	OperationID string
	AutoGroups  []string
	Rotation    *CanvasManagedTokenRotation
}

// CanvasManagedTokenRotation 绑定调用方已排空任务的原 Token、版本和完整 Key 的 SHA-256。
// 正常轮换保留 Token ID 及账务归属；不接受人工修改后的 Key，也不恢复撤销 Token。
type CanvasManagedTokenRotation struct {
	TokenID            int    `json:"token_id"`
	CredentialRevision int64  `json:"credential_revision"`
	KeyFingerprint     string `json:"key_fingerprint"`
}

// CanvasManagedTokenRepair 描述显式修复已删除 Token 所需的旧身份事实。
// 修复只接受原 managed 行的 Token ID 与 credential revision，不接受旧 Key。
type CanvasManagedTokenRepair struct {
	TokenID            int   `json:"token_id"`
	CredentialRevision int64 `json:"credential_revision"`
}

// CanvasManagedTokenRepairInput 描述受保护的 Token 修复请求。
// AutoGroups 是当前用户权限过滤后的显式范围，Repair 绑定待修复的旧 Token。
type CanvasManagedTokenRepairInput struct {
	GrantID     string
	GroupID     string
	OperationID string
	AutoGroups  []string
	Repair      CanvasManagedTokenRepair
}

// GetCanvasManagedTokenRepairTarget 读取本人 grant 与原分组绑定的非敏感修复事实。
// 仅在 Token 缺失或被人工改组且其它固定合同仍有效时返回；其它状态一律拒绝。
func GetCanvasManagedTokenRepairTarget(grantID, groupID string) (*CanvasManagedTokenRepair, error) {
	grantID = strings.TrimSpace(grantID)
	groupID = strings.TrimSpace(groupID)
	if grantID == "" || groupID == "" {
		return nil, ErrCanvasManagedTokenChanged
	}
	var managed CanvasManagedToken
	if err := DB.Where("grant_id = ? AND group_id = ?", grantID, groupID).First(&managed).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCanvasManagedTokenChanged
		}
		return nil, err
	}
	if managed.Status != CanvasManagedTokenStatusActive || managed.ReplacedByID != nil {
		return nil, ErrCanvasManagedTokenChanged
	}
	var token Token
	err := DB.Unscoped().Where("id = ? AND user_id = ?", managed.TokenID, managed.UserID).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && token.DeletedAt.Valid) {
		return &CanvasManagedTokenRepair{TokenID: managed.TokenID, CredentialRevision: managed.CredentialRevision}, nil
	}
	if err != nil {
		return nil, err
	}
	if token.Status == common.TokenStatusEnabled && token.ExpiredTime > time.Now().Unix() &&
		token.Group != managed.GroupID && canvasManagedTokenContractValidIgnoringGroup(&managed, &token) {
		return &CanvasManagedTokenRepair{TokenID: managed.TokenID, CredentialRevision: managed.CredentialRevision}, nil
	}
	return nil, ErrCanvasManagedTokenChanged
}

// CanvasAcceptanceInput 描述供应商调用受理前必须匹配的冻结授权事实。
type CanvasAcceptanceInput struct {
	TokenID                    int
	Issuer                     string
	ExpectedIssuer             string
	ExpectedUserID             string
	GrantID                    string
	InstanceID                 string
	ExpectedTokenID            string
	ExpectedPolicyGroup        string
	ExpectedPermissionRevision string
	ExpectedAutoGroups         []string
	ActualGroup                string
	Model                      string
	ChannelID                  int
}

// canvasPermissionOptions 保存会影响管理 Token 资格的站点配置语义。
// 分组倍率只保留键，不保存价格数值，避免单纯调价递增权限修订。
type canvasPermissionOptions struct {
	UserUsableGroups       []string `json:"user_usable_groups"`
	LegacyRatioGroups      []string `json:"legacy_ratio_groups"`
	ConfiguredRatioGroups  []string `json:"configured_ratio_groups"`
	SpecialGroupDirectives []string `json:"special_group_directives"`
	MaxTokenAutoGroups     string   `json:"max_token_auto_groups,omitempty"`
}

// CanvasGrantTokenHash 以用途隔离的 HMAC 摘要 grant token，避免数据库泄露可用凭据。
func CanvasGrantTokenHash(raw string) string {
	return common.GenerateHMACWithKey([]byte("canvas-grant-v1:"+common.SessionSecret), raw)
}

// UpsertCanvasGrantWithTx 在授权码消费事务内创建或轮换唯一 grant。
// 重新授权沿用 grant ID，以便已有管理关系可恢复，但旧 Bearer token 立即失效。
func UpsertCanvasGrantWithTx(tx *gorm.DB, clientID, instanceID string, userID int, tokenHash, scopes string, expiresAt int64) (*CanvasGrant, error) {
	clientID = strings.TrimSpace(clientID)
	instanceID = strings.TrimSpace(instanceID)
	if tx == nil || clientID == "" || len(clientID) > 64 || instanceID == "" || len(instanceID) > 128 || userID <= 0 || len(tokenHash) != 64 || scopes == "" || expiresAt <= time.Now().Unix() {
		return nil, ErrCanvasGrantInvalid
	}
	now := time.Now().Unix()
	var grant CanvasGrant
	err := lockForUpdate(tx).Where("client_id = ? AND instance_id = ? AND user_id = ?", clientID, instanceID, userID).First(&grant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		grant = CanvasGrant{
			ID: uuid.NewString(), ClientID: clientID, InstanceID: instanceID, UserID: userID,
			TokenHash: tokenHash, Scopes: scopes, Status: CanvasGrantStatusActive,
			Revision: 1, IssuedAt: now, ExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&grant).Error; err != nil {
			return nil, err
		}
		return &grant, nil
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"token_hash": tokenHash, "scopes": scopes, "status": CanvasGrantStatusActive,
		"revision":  gorm.Expr("revision + ?", 1),
		"issued_at": now, "expires_at": expiresAt, "revoked_at": int64(0), "updated_at": now,
	}
	if err := tx.Model(&CanvasGrant{}).Where("id = ?", grant.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := tx.First(&grant, "id = ?", grant.ID).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

// GetCanvasGrantByToken 从主数据库读取 grant 和用户权威状态，不依赖 Redis 热缓存。
func GetCanvasGrantByToken(raw string) (*CanvasGrant, *User, error) {
	if len(raw) < 16 || len(raw) > 512 {
		return nil, nil, ErrCanvasGrantInvalid
	}
	var grant CanvasGrant
	if err := DB.Where("token_hash = ?", CanvasGrantTokenHash(raw)).First(&grant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrCanvasGrantInvalid
		}
		return nil, nil, err
	}
	var user User
	if err := DB.First(&user, grant.UserID).Error; err != nil {
		return nil, nil, err
	}
	if grant.Status != CanvasGrantStatusActive || grant.ExpiresAt <= time.Now().Unix() || user.Status != common.UserStatusEnabled {
		return nil, nil, ErrCanvasGrantInvalid
	}
	return &grant, &user, nil
}

// EnsureCanvasManagedToken 以 grant、分组和 operation_id 原子创建或恢复管理 Token。
// 普通分组固定实际组；Auto 必须持久化非空显式范围且关闭跨组重试。
func EnsureCanvasManagedToken(input CanvasManagedTokenInput) (*CanvasManagedAuthority, error) {
	input.GrantID = strings.TrimSpace(input.GrantID)
	input.GroupID = strings.TrimSpace(input.GroupID)
	input.OperationID = strings.TrimSpace(input.OperationID)
	if input.GrantID == "" || input.GroupID == "" || len(input.GroupID) > 191 || !canvasOperationIDPattern.MatchString(input.OperationID) {
		return nil, ErrCanvasIdempotencyConflict
	}
	if input.GroupID == CanvasExcludedGroup {
		return nil, ErrCanvasGroupExcluded
	}
	input.AutoGroups = normalizeCanvasAutoGroups(input.AutoGroups)
	if input.GroupID == "auto" && len(input.AutoGroups) == 0 {
		return nil, ErrCanvasAutoGroupEmpty
	}
	if input.GroupID != "auto" {
		input.AutoGroups = nil
	}
	digest := canvasOperationDigest(input.GrantID, input.GroupID)
	if rotation := input.Rotation; rotation != nil {
		fingerprint, err := hex.DecodeString(rotation.KeyFingerprint)
		if rotation.TokenID <= 0 || rotation.CredentialRevision < 1 || rotation.CredentialRevision >= 9007199254740991 || err != nil || len(fingerprint) != sha256.Size {
			return nil, ErrCanvasIdempotencyConflict
		}
		encoded, err := common.Marshal(rotation)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256([]byte("rotate:" + digest + ":" + string(encoded)))
		digest = hex.EncodeToString(hash[:])
	}
	var managedID int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 第一条写语句在 SQLite 获取写锁；在 MySQL/PostgreSQL 也串行化同一 grant 的并发创建。
		result := tx.Model(&CanvasGrant{}).Where("id = ?", input.GrantID).UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if result.Error != nil {
			return result.Error
		}
		var grant CanvasGrant
		if err := lockForUpdate(tx).First(&grant, "id = ?", input.GrantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCanvasGrantInvalid
			}
			return err
		}
		if grant.Status != CanvasGrantStatusActive || grant.ExpiresAt <= time.Now().Unix() {
			return ErrCanvasGrantInvalid
		}
		var operation CanvasIdempotencyOperation
		err := tx.Where("grant_id = ? AND operation_id = ?", grant.ID, input.OperationID).First(&operation).Error
		if err == nil {
			if operation.RequestDigest != digest {
				return ErrCanvasIdempotencyConflict
			}
			var managed CanvasManagedToken
			if err := lockForUpdate(tx).First(&managed, operation.ManagedTokenID).Error; err != nil {
				return err
			}
			if rotation := input.Rotation; rotation != nil && (managed.Status != CanvasManagedTokenStatusActive || managed.TokenID != rotation.TokenID || managed.CredentialRevision != rotation.CredentialRevision+1) {
				return ErrCanvasManagedTokenChanged
			}
			if err := validateCanvasManagedTokenWithTx(tx, &grant, &managed, input.AutoGroups); err != nil {
				return err
			}
			managedID = managed.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var managed CanvasManagedToken
		err = lockForUpdate(tx).Where("grant_id = ? AND group_id = ?", grant.ID, input.GroupID).First(&managed).Error
		if err == nil {
			if input.Rotation != nil && managed.Status != CanvasManagedTokenStatusActive {
				return ErrCanvasManagedTokenChanged
			}
			if err := validateCanvasManagedTokenWithTx(tx, &grant, &managed, input.AutoGroups); err != nil {
				return err
			}
			if rotation := input.Rotation; rotation != nil {
				var token Token
				if err := lockForUpdate(tx).First(&token, managed.TokenID).Error; err != nil {
					return err
				}
				fingerprint := sha256.Sum256([]byte(token.GetFullKey()))
				if managed.TokenID != rotation.TokenID || managed.CredentialRevision != rotation.CredentialRevision || hex.EncodeToString(fingerprint[:]) != rotation.KeyFingerprint {
					return ErrCanvasManagedTokenChanged
				}
				key, err := common.GenerateKey()
				if err != nil {
					return err
				}
				if err := invalidateTokenCacheForMutation(token.Key); err != nil {
					return fmt.Errorf("invalidate rotating token cache: %w", err)
				}
				if err := tx.Model(&Token{}).Where("id = ?", token.Id).Update("key", key).Error; err != nil {
					return err
				}
				managed.CredentialRevision++
				newFingerprint := sha256.Sum256([]byte(key))
				if err := tx.Model(&managed).Updates(map[string]any{"credential_revision": managed.CredentialRevision, "key_fingerprint": hex.EncodeToString(newFingerprint[:]), "updated_at": time.Now().Unix()}).Error; err != nil {
					return err
				}
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			if input.Rotation != nil {
				return ErrCanvasManagedTokenChanged
			}
			created, createErr := createCanvasManagedTokenWithTx(tx, &grant, input.GroupID, input.AutoGroups)
			if createErr != nil {
				return createErr
			}
			managed = *created
		} else {
			return err
		}

		operation = CanvasIdempotencyOperation{
			GrantID: grant.ID, OperationID: input.OperationID, RequestDigest: digest,
			ManagedTokenID: managed.ID, CreatedAt: time.Now().Unix(),
		}
		if err := tx.Create(&operation).Error; err != nil {
			return err
		}
		managedID = managed.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	authority, err := ReconcileCanvasManagedAuthority(managedID)
	if err == nil && input.Rotation != nil && authority.Managed.CredentialRevision != input.Rotation.CredentialRevision+1 {
		return nil, ErrCanvasManagedTokenChanged
	}
	return authority, err
}

// RepairCanvasManagedToken 显式修复已删除或被人工改组的管理 Token。
// 缺失 Token 会签发新 ID/Key；仅分组变化会保留 ID/Key；两种路径都递增凭据修订。
// 修复不复活禁用、改权限或已撤销关系，且通过独立的 operation digest 幂等。
func RepairCanvasManagedToken(input CanvasManagedTokenRepairInput) (*CanvasManagedAuthority, error) {
	input.GrantID = strings.TrimSpace(input.GrantID)
	input.GroupID = strings.TrimSpace(input.GroupID)
	input.OperationID = strings.TrimSpace(input.OperationID)
	if input.GrantID == "" || input.GroupID == "" || len(input.GroupID) > 191 || !canvasOperationIDPattern.MatchString(input.OperationID) ||
		input.Repair.TokenID <= 0 || input.Repair.CredentialRevision < 1 || input.Repair.CredentialRevision >= 9007199254740991 {
		return nil, ErrCanvasIdempotencyConflict
	}
	if input.GroupID == CanvasExcludedGroup {
		return nil, ErrCanvasGroupExcluded
	}
	input.AutoGroups = normalizeCanvasAutoGroups(input.AutoGroups)
	if input.GroupID == "auto" && len(input.AutoGroups) == 0 {
		return nil, ErrCanvasAutoGroupEmpty
	}
	if input.GroupID != "auto" {
		input.AutoGroups = nil
	}
	digest := canvasRepairOperationDigest(input)
	var managedID int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&CanvasGrant{}).Where("id = ?", input.GrantID).UpdateColumn("updated_at", gorm.Expr("updated_at"))
		if result.Error != nil {
			return result.Error
		}
		var grant CanvasGrant
		if err := lockForUpdate(tx).First(&grant, "id = ?", input.GrantID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCanvasGrantInvalid
			}
			return err
		}
		if grant.Status != CanvasGrantStatusActive || grant.ExpiresAt <= time.Now().Unix() {
			return ErrCanvasGrantInvalid
		}

		var operation CanvasIdempotencyOperation
		err := tx.Where("grant_id = ? AND operation_id = ?", grant.ID, input.OperationID).First(&operation).Error
		if err == nil {
			if operation.RequestDigest != digest {
				return ErrCanvasIdempotencyConflict
			}
			var managed CanvasManagedToken
			if err := lockForUpdate(tx).First(&managed, operation.ManagedTokenID).Error; err != nil {
				return err
			}
			if err := validateCanvasRepairReplayWithTx(tx, &grant, &managed, input); err != nil {
				return err
			}
			managedID = managed.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var managed CanvasManagedToken
		if err := lockForUpdate(tx).Where("grant_id = ? AND group_id = ?", grant.ID, input.GroupID).First(&managed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCanvasManagedTokenChanged
			}
			return err
		}
		if managed.UserID != grant.UserID || managed.ReplacedByID != nil || managed.Status != CanvasManagedTokenStatusActive ||
			managed.TokenID != input.Repair.TokenID || managed.CredentialRevision != input.Repair.CredentialRevision {
			return ErrCanvasManagedTokenChanged
		}

		token, repairKind, err := loadCanvasRepairTokenWithTx(tx, &managed)
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		if repairKind == canvasManagedRepairGroup && token != nil && token.ExpiredTime != grant.ExpiresAt && grant.IssuedAt < managed.UpdatedAt {
			return ErrCanvasManagedTokenChanged
		}
		switch repairKind {
		case canvasManagedRepairMissing:
			if token != nil && token.Key != "" {
				if err := invalidateTokenCacheForMutation(token.Key); err != nil {
					return fmt.Errorf("invalidate deleted token cache: %w", err)
				}
			}
			replacement, createErr := createCanvasManagedReplacementTokenWithTx(tx, &grant, input.GroupID, input.AutoGroups)
			if createErr != nil {
				return createErr
			}
			fingerprint := sha256.Sum256([]byte(replacement.GetFullKey()))
			managed.TokenID = replacement.Id
			managed.CredentialRevision++
			managed.KeyFingerprint = hex.EncodeToString(fingerprint[:])
			managed.AutoGroups = replacement.AutoGroups
			if err := tx.Model(&CanvasManagedToken{}).Where("id = ?", managed.ID).Updates(map[string]any{
				"token_id": managed.TokenID, "credential_revision": managed.CredentialRevision,
				"key_fingerprint": managed.KeyFingerprint, "auto_groups": managed.AutoGroups,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
		case canvasManagedRepairGroup:
			if token == nil {
				return ErrCanvasManagedTokenChanged
			}
			if err := invalidateTokenCacheForMutation(token.Key); err != nil {
				return fmt.Errorf("invalidate repairing token cache: %w", err)
			}
			tokenUpdates := map[string]any{"group": managed.GroupID, "expired_time": grant.ExpiresAt}
			if err := token.SetAutoGroups(input.AutoGroups); err != nil {
				return err
			}
			tokenUpdates["auto_groups"] = token.AutoGroups
			managed.AutoGroups = token.AutoGroups
			if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, grant.UserID).Updates(tokenUpdates).Error; err != nil {
				return err
			}
			managed.CredentialRevision++
			fingerprint := sha256.Sum256([]byte(token.GetFullKey()))
			managed.KeyFingerprint = hex.EncodeToString(fingerprint[:])
			if err := tx.Model(&CanvasManagedToken{}).Where("id = ?", managed.ID).Updates(map[string]any{
				"credential_revision": managed.CredentialRevision, "key_fingerprint": managed.KeyFingerprint,
				"auto_groups": managed.AutoGroups, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		default:
			return ErrCanvasManagedTokenChanged
		}

		operation = CanvasIdempotencyOperation{
			GrantID: grant.ID, OperationID: input.OperationID, RequestDigest: digest,
			ManagedTokenID: managed.ID, CreatedAt: now,
		}
		if err := tx.Create(&operation).Error; err != nil {
			return err
		}
		managedID = managed.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	authority, err := ReconcileCanvasManagedAuthority(managedID)
	if err != nil {
		return nil, err
	}
	if authority.Managed.CredentialRevision != input.Repair.CredentialRevision+1 {
		return nil, ErrCanvasManagedTokenChanged
	}
	return authority, nil
}

type canvasManagedRepairKind uint8

const (
	canvasManagedRepairNone canvasManagedRepairKind = iota
	canvasManagedRepairMissing
	canvasManagedRepairGroup
)

// loadCanvasRepairTokenWithTx 区分软删除/缺失和单独的人工改组。
// 读取使用 Unscoped 仅为确认删除事实，不会把已删除 Token 重新启用。
func loadCanvasRepairTokenWithTx(tx *gorm.DB, managed *CanvasManagedToken) (*Token, canvasManagedRepairKind, error) {
	var token Token
	err := lockForUpdate(tx.Unscoped()).First(&token, "id = ? AND user_id = ?", managed.TokenID, managed.UserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, canvasManagedRepairMissing, nil
	}
	if err != nil {
		return nil, canvasManagedRepairNone, err
	}
	if token.DeletedAt.Valid {
		return &token, canvasManagedRepairMissing, nil
	}
	if token.Status != common.TokenStatusEnabled || token.ExpiredTime <= time.Now().Unix() {
		return &token, canvasManagedRepairNone, nil
	}
	if token.Group != managed.GroupID && canvasManagedTokenContractValidIgnoringGroup(managed, &token) {
		return &token, canvasManagedRepairGroup, nil
	}
	return &token, canvasManagedRepairNone, nil
}

// validateCanvasRepairReplayWithTx 只接受原修复结果仍保持预期版本的幂等重试。
func validateCanvasRepairReplayWithTx(tx *gorm.DB, grant *CanvasGrant, managed *CanvasManagedToken, input CanvasManagedTokenRepairInput) error {
	if managed.GrantID != grant.ID || managed.GroupID != input.GroupID || managed.UserID != grant.UserID || managed.ReplacedByID != nil || managed.Status != CanvasManagedTokenStatusActive ||
		managed.CredentialRevision != input.Repair.CredentialRevision+1 {
		return ErrCanvasManagedTokenChanged
	}
	var token Token
	if err := tx.First(&token, "id = ? AND user_id = ?", managed.TokenID, grant.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCanvasManagedTokenMissing
		}
		return err
	}
	if token.Status != common.TokenStatusEnabled || token.ExpiredTime <= time.Now().Unix() || !canvasManagedTokenContractValid(managed, &token) {
		return ErrCanvasManagedTokenChanged
	}
	return nil
}

// createCanvasManagedReplacementTokenWithTx 只创建新 Token，不创建第二条 managed 关系。
func createCanvasManagedReplacementTokenWithTx(tx *gorm.DB, grant *CanvasGrant, group string, autoGroups []string) (*Token, error) {
	var user User
	if err := lockForUpdate(tx).First(&user, grant.UserID).Error; err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, ErrCanvasGrantInvalid
	}
	var count int64
	if err := tx.Model(&Token{}).Where("user_id = ?", grant.UserID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= int64(operation_setting.GetMaxUserTokens()) {
		return nil, ErrCanvasTokenLimitReached
	}
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	token := &Token{
		UserId: grant.UserID, Key: key, Status: common.TokenStatusEnabled,
		Name: canvasManagedTokenName(grant.InstanceID, group), CreatedTime: now, AccessedTime: now,
		ExpiredTime: grant.ExpiresAt, UnlimitedQuota: true, Group: group, CrossGroupRetry: false,
	}
	if group == "auto" {
		if err := token.SetAutoGroups(autoGroups); err != nil {
			return nil, err
		}
	}
	if err := tx.Create(token).Error; err != nil {
		return nil, err
	}
	return token, nil
}

// ReconcileCanvasManagedAuthority 读取权威表并在权限指纹变化时单调递增修订。
// 指纹覆盖用户、grant、Token、相关站点选项、Ability 和渠道状态。
func ReconcileCanvasManagedAuthority(managedID int64) (*CanvasManagedAuthority, error) {
	initCol()
	var authority CanvasManagedAuthority
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&authority.Managed, managedID).Error; err != nil {
			return err
		}
		if err := loadCanvasManagedAuthorityWithTx(tx, &authority); err != nil {
			return err
		}
		fingerprint, err := canvasPermissionFingerprintWithTx(tx, &authority)
		if err != nil {
			return err
		}
		if authority.Managed.PermissionFingerprint != fingerprint {
			authority.Managed.PermissionFingerprint = fingerprint
			authority.Managed.PermissionRevision++
			authority.Managed.UpdatedAt = time.Now().Unix()
			if err := tx.Model(&CanvasManagedToken{}).Where("id = ?", authority.Managed.ID).Updates(map[string]any{
				"permission_fingerprint": fingerprint,
				"permission_revision":    authority.Managed.PermissionRevision,
				"updated_at":             authority.Managed.UpdatedAt,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &authority, nil
}

// ValidateCanvasAcceptance 在预扣和供应商 POST 前验证冻结的管理授权。
// 返回 false 表示当前 Token 不是 Canvas 管理 Token，普通调用保持原合同。
func ValidateCanvasAcceptance(input CanvasAcceptanceInput) (bool, *CanvasManagedAuthority, error) {
	var managed CanvasManagedToken
	err := DB.Where("token_id = ?", input.TokenID).First(&managed).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil, nil
	}
	if err != nil {
		return true, nil, err
	}
	authority, err := ReconcileCanvasManagedAuthority(managed.ID)
	if err != nil {
		return true, nil, err
	}
	if input.Issuer == "" || input.ExpectedIssuer == "" || input.ExpectedUserID == "" || input.GrantID == "" || input.InstanceID == "" || input.ExpectedTokenID == "" || input.ExpectedPolicyGroup == "" || input.ExpectedPermissionRevision == "" {
		return true, authority, ErrCanvasAcceptanceMismatch
	}
	if input.Issuer != input.ExpectedIssuer || strconv.Itoa(authority.User.Id) != input.ExpectedUserID || authority.Grant.ID != input.GrantID || authority.Grant.InstanceID != input.InstanceID || strconv.Itoa(authority.Token.Id) != input.ExpectedTokenID || authority.Managed.GroupID != input.ExpectedPolicyGroup {
		return true, authority, ErrCanvasAcceptanceMismatch
	}
	if input.ExpectedPermissionRevision != strconv.FormatInt(authority.Managed.PermissionRevision, 10) {
		return true, authority, ErrCanvasPermissionRevisionConflict
	}
	if input.ActualGroup == "" || input.ActualGroup == CanvasExcludedGroup {
		return true, authority, ErrCanvasAcceptanceMismatch
	}
	if authority.Managed.GroupID == "auto" {
		groups, parseErr := authority.Token.GetAutoGroups()
		if parseErr != nil || !equalStringSlices(groups, input.ExpectedAutoGroups) || !slicesContainsString(groups, input.ActualGroup) {
			return true, authority, ErrCanvasAcceptanceMismatch
		}
	} else if len(input.ExpectedAutoGroups) != 0 || input.ActualGroup != authority.Managed.GroupID {
		return true, authority, ErrCanvasAcceptanceMismatch
	}
	if input.Model == "" || input.ChannelID <= 0 {
		return true, authority, ErrCanvasAcceptanceMismatch
	}
	var count int64
	abilityQuery := DB.Table("abilities").Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities."+commonGroupCol+" = ? AND abilities.model = ? AND abilities.channel_id = ? AND abilities.enabled = ? AND channels.status = ?",
			input.ActualGroup, input.Model, input.ChannelID, true, common.ChannelStatusEnabled)
	if err := abilityQuery.Count(&count).Error; err != nil {
		return true, authority, err
	}
	if count != 1 {
		return true, authority, ErrCanvasGroupUnavailable
	}
	return true, authority, nil
}

// RevokeCanvasGrant 撤销 grant 并只禁用该 grant 记录归属的派生 Token。
// Token 缓存无法先行失效时事务回滚，调用方可安全重试原撤销请求。
func RevokeCanvasGrant(raw string) (*CanvasGrant, error) {
	if len(raw) < 16 || len(raw) > 512 {
		return nil, ErrCanvasGrantInvalid
	}
	var revoked CanvasGrant
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("token_hash = ?", CanvasGrantTokenHash(raw)).First(&revoked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCanvasGrantInvalid
			}
			return err
		}
		if revoked.Status == CanvasGrantStatusRevoked {
			return nil
		}
		var managed []CanvasManagedToken
		if err := tx.Where("grant_id = ? AND status = ?", revoked.ID, CanvasManagedTokenStatusActive).Find(&managed).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		for i := range managed {
			var token Token
			if err := tx.First(&token, "id = ? AND user_id = ?", managed[i].TokenID, revoked.UserID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if err := tx.Model(&CanvasManagedToken{}).Where("id = ?", managed[i].ID).Updates(map[string]any{
						"status": CanvasManagedTokenStatusChanged, "revoked_at": now, "updated_at": now,
					}).Error; err != nil {
						return err
					}
					continue
				}
				return err
			}
			managedStatus := CanvasManagedTokenStatusRevoked
			if token.Status != common.TokenStatusEnabled || !canvasManagedTokenContractValid(&managed[i], &token) {
				managedStatus = CanvasManagedTokenStatusChanged
			}
			if token.Status == common.TokenStatusEnabled {
				if err := invalidateTokenCacheForMutation(token.Key); err != nil {
					return fmt.Errorf("invalidate managed token cache: %w", err)
				}
				if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, revoked.UserID).Update("status", common.TokenStatusDisabled).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&CanvasManagedToken{}).Where("id = ?", managed[i].ID).Updates(map[string]any{
				"status": managedStatus, "revoked_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&CanvasGrant{}).Where("id = ?", revoked.ID).Updates(map[string]any{
			"status": CanvasGrantStatusRevoked, "revoked_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		revoked.Status, revoked.RevokedAt, revoked.UpdatedAt = CanvasGrantStatusRevoked, now, now
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &revoked, nil
}

// createCanvasManagedTokenWithTx 在调用方事务内创建固定权限的专用 Token 和归属关系。
func createCanvasManagedTokenWithTx(tx *gorm.DB, grant *CanvasGrant, group string, autoGroups []string) (*CanvasManagedToken, error) {
	var user User
	if err := lockForUpdate(tx).First(&user, grant.UserID).Error; err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, ErrCanvasGrantInvalid
	}
	var count int64
	if err := tx.Model(&Token{}).Where("user_id = ?", grant.UserID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= int64(operation_setting.GetMaxUserTokens()) {
		return nil, ErrCanvasTokenLimitReached
	}
	key, err := common.GenerateKey()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	token := Token{
		UserId: grant.UserID, Key: key, Status: common.TokenStatusEnabled,
		Name: canvasManagedTokenName(grant.InstanceID, group), CreatedTime: now, AccessedTime: now,
		ExpiredTime: grant.ExpiresAt, UnlimitedQuota: true, Group: group, CrossGroupRetry: false,
	}
	if group == "auto" {
		if err := token.SetAutoGroups(autoGroups); err != nil {
			return nil, err
		}
	}
	if err := tx.Create(&token).Error; err != nil {
		return nil, err
	}
	managed := CanvasManagedToken{
		GrantID: grant.ID, UserID: grant.UserID, GroupID: group, TokenID: token.Id,
		CredentialRevision: 1, KeyFingerprint: canvasTokenKeyFingerprint(token.GetFullKey()),
		PermissionRevision: 0, Status: CanvasManagedTokenStatusActive,
		CreatedAt: now, UpdatedAt: now,
	}
	if group == "auto" {
		managed.AutoGroups = token.AutoGroups
	}
	if err := tx.Create(&managed).Error; err != nil {
		return nil, err
	}
	return &managed, nil
}

// canvasTokenKeyFingerprint 计算管理 Token 当前完整 Key 的固定指纹。
// 指纹用于识别人工换 Key，而不会把可用 Key 写入管理关系或日志。
func canvasTokenKeyFingerprint(key string) string {
	fingerprint := sha256.Sum256([]byte(key))
	return hex.EncodeToString(fingerprint[:])
}

// validateCanvasManagedTokenWithTx 校验现有关系，并只恢复未被人工修改的撤销 Token。
func validateCanvasManagedTokenWithTx(tx *gorm.DB, grant *CanvasGrant, managed *CanvasManagedToken, autoGroups []string) error {
	if managed.UserID != grant.UserID || managed.ReplacedByID != nil {
		return ErrCanvasManagedTokenChanged
	}
	var token Token
	if err := tx.First(&token, "id = ? AND user_id = ?", managed.TokenID, grant.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCanvasManagedTokenMissing
		}
		return err
	}
	if token.Status == common.TokenStatusEnabled && token.ExpiredTime > time.Now().Unix() &&
		token.Group != managed.GroupID && canvasManagedTokenContractValidIgnoringGroup(managed, &token) {
		return ErrCanvasManagedTokenGroupMismatch
	}
	if !canvasManagedTokenContractValid(managed, &token) {
		return ErrCanvasManagedTokenChanged
	}
	storedAutoGroups, err := token.GetAutoGroups()
	if err != nil {
		return ErrCanvasManagedTokenChanged
	}
	managedAutoGroups, err := canvasManagedAutoGroups(managed)
	if err != nil || !equalStringSlices(storedAutoGroups, managedAutoGroups) {
		return ErrCanvasManagedTokenChanged
	}
	if managed.GroupID != "auto" && (len(storedAutoGroups) != 0 || len(autoGroups) != 0) {
		return ErrCanvasManagedTokenChanged
	}
	restore := managed.Status == CanvasManagedTokenStatusRevoked && token.Status == common.TokenStatusDisabled
	if !restore && (managed.Status != CanvasManagedTokenStatusActive || token.Status != common.TokenStatusEnabled) {
		return ErrCanvasManagedTokenChanged
	}
	updateScope := managed.GroupID == "auto" && !equalStringSlices(managedAutoGroups, autoGroups)
	updateExpiry := token.ExpiredTime != grant.ExpiresAt
	if updateExpiry && !restore && grant.IssuedAt < managed.UpdatedAt {
		return ErrCanvasManagedTokenChanged
	}
	if !restore && !updateScope && !updateExpiry {
		return nil
	}
	if err := invalidateTokenCacheForMutation(token.Key); err != nil {
		return fmt.Errorf("invalidate managed token cache: %w", err)
	}
	now := time.Now().Unix()
	tokenUpdates := map[string]any{"expired_time": grant.ExpiresAt}
	managedUpdates := map[string]any{"updated_at": now}
	if restore {
		tokenUpdates["status"] = common.TokenStatusEnabled
		managedUpdates["status"] = CanvasManagedTokenStatusActive
		managedUpdates["revoked_at"] = int64(0)
		managed.Status, managed.RevokedAt = CanvasManagedTokenStatusActive, 0
	}
	if updateScope {
		if err := token.SetAutoGroups(autoGroups); err != nil {
			return err
		}
		managed.AutoGroups = token.AutoGroups
		tokenUpdates["auto_groups"] = token.AutoGroups
		managedUpdates["auto_groups"] = managed.AutoGroups
	}
	if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", token.Id, grant.UserID).Updates(tokenUpdates).Error; err != nil {
		return err
	}
	if err := tx.Model(&CanvasManagedToken{}).Where("id = ?", managed.ID).Updates(managedUpdates).Error; err != nil {
		return err
	}
	managed.UpdatedAt = now
	return nil
}

// loadCanvasManagedAuthorityWithTx 锁定并装载一次管理 Token 校验所需的全部权威记录。
func loadCanvasManagedAuthorityWithTx(tx *gorm.DB, authority *CanvasManagedAuthority) error {
	if authority.Managed.Status != CanvasManagedTokenStatusActive || authority.Managed.ReplacedByID != nil {
		return ErrCanvasManagedTokenChanged
	}
	if err := lockForUpdate(tx).First(&authority.Grant, "id = ?", authority.Managed.GrantID).Error; err != nil {
		return err
	}
	if err := lockForUpdate(tx).First(&authority.User, authority.Managed.UserID).Error; err != nil {
		return err
	}
	if err := lockForUpdate(tx).First(&authority.Token, "id = ? AND user_id = ?", authority.Managed.TokenID, authority.Managed.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCanvasManagedTokenMissing
		}
		return err
	}
	if authority.Grant.Status != CanvasGrantStatusActive || authority.Grant.ExpiresAt <= time.Now().Unix() || authority.User.Status != common.UserStatusEnabled {
		return ErrCanvasGrantInvalid
	}
	if authority.Grant.UserID != authority.Managed.UserID {
		return ErrCanvasManagedTokenChanged
	}
	if authority.Token.Status == common.TokenStatusEnabled && authority.Token.ExpiredTime > time.Now().Unix() &&
		authority.Token.Group != authority.Managed.GroupID && canvasManagedTokenContractValidIgnoringGroup(&authority.Managed, &authority.Token) {
		return ErrCanvasManagedTokenGroupMismatch
	}
	if authority.Token.Status != common.TokenStatusEnabled || authority.Token.ExpiredTime <= time.Now().Unix() || !canvasManagedTokenContractValid(&authority.Managed, &authority.Token) {
		return ErrCanvasManagedTokenChanged
	}
	if authority.Managed.GroupID == CanvasExcludedGroup {
		return ErrCanvasGroupExcluded
	}
	groups, err := authority.Token.GetAutoGroups()
	if err != nil {
		return ErrCanvasManagedTokenChanged
	}
	managedGroups, err := canvasManagedAutoGroups(&authority.Managed)
	if err != nil || !equalStringSlices(groups, managedGroups) {
		return ErrCanvasManagedTokenChanged
	}
	if authority.Managed.GroupID == "auto" && (len(groups) == 0 || slicesContainsString(groups, CanvasExcludedGroup)) {
		return ErrCanvasAutoGroupEmpty
	}
	if authority.Managed.GroupID != "auto" && len(groups) != 0 {
		return ErrCanvasManagedTokenChanged
	}
	return nil
}

// canvasPermissionFingerprintWithTx 计算排除余额和价格后的权限指纹。
func canvasPermissionFingerprintWithTx(tx *gorm.DB, authority *CanvasManagedAuthority) (string, error) {
	groups := []string{authority.Managed.GroupID}
	if authority.Managed.GroupID == "auto" {
		var err error
		groups, err = authority.Token.GetAutoGroups()
		if err != nil || len(groups) == 0 {
			return "", ErrCanvasAutoGroupEmpty
		}
	}
	groups = normalizeCanvasAutoGroups(groups)
	sort.Strings(groups)

	permissionOptions, err := canvasPermissionOptionsWithTx(tx, authority)
	if err != nil {
		return "", err
	}
	type permissionAbility struct {
		Group         string  `json:"group" gorm:"column:group_id"`
		Model         string  `json:"model"`
		ChannelID     int     `json:"channel_id"`
		Enabled       bool    `json:"enabled"`
		Priority      *int64  `json:"priority"`
		Weight        uint    `json:"weight"`
		Tag           *string `json:"tag"`
		ChannelStatus int     `json:"channel_status"`
	}
	abilities := make([]permissionAbility, 0)
	if len(groups) > 0 {
		if err := tx.Table("abilities").Select("abilities."+commonGroupCol+" AS group_id, abilities.model, abilities.channel_id, abilities.enabled, abilities.priority, abilities.weight, abilities.tag, channels.status AS channel_status").
			Joins("LEFT JOIN channels ON channels.id = abilities.channel_id").
			Where("abilities."+commonGroupCol+" IN ?", groups).
			Order("abilities." + commonGroupCol + " ASC, abilities.model ASC, abilities.channel_id ASC").Scan(&abilities).Error; err != nil {
			return "", err
		}
	}
	payload := struct {
		Grant struct {
			ID, ClientID, InstanceID, Status string
			Revision, ExpiresAt              int64
		}
		User struct {
			ID, Status  int
			Group       string
			AuthVersion int64
		}
		Managed struct {
			ID, CredentialRevision      int64
			GroupID, AutoGroups, Status string
			TokenID                     int
		}
		Token struct {
			ID, Status                          int
			ExpiredTime                         int64
			Group, ModelLimits, AutoGroups      string
			ModelLimitsEnabled, CrossGroupRetry bool
		}
		Options   canvasPermissionOptions
		Abilities []permissionAbility
	}{}
	payload.Grant.ID, payload.Grant.ClientID, payload.Grant.InstanceID = authority.Grant.ID, authority.Grant.ClientID, authority.Grant.InstanceID
	payload.Grant.Status, payload.Grant.Revision, payload.Grant.ExpiresAt = authority.Grant.Status, authority.Grant.Revision, authority.Grant.ExpiresAt
	payload.User.ID, payload.User.Status, payload.User.Group, payload.User.AuthVersion = authority.User.Id, authority.User.Status, authority.User.Group, authority.User.AuthVersion
	payload.Managed.ID, payload.Managed.CredentialRevision = authority.Managed.ID, authority.Managed.CredentialRevision
	payload.Managed.GroupID, payload.Managed.AutoGroups, payload.Managed.Status, payload.Managed.TokenID = authority.Managed.GroupID, authority.Managed.AutoGroups, authority.Managed.Status, authority.Managed.TokenID
	payload.Token.ID, payload.Token.Status, payload.Token.ExpiredTime = authority.Token.Id, authority.Token.Status, authority.Token.ExpiredTime
	payload.Token.Group, payload.Token.ModelLimits, payload.Token.AutoGroups = authority.Token.Group, authority.Token.ModelLimits, authority.Token.AutoGroups
	payload.Token.ModelLimitsEnabled, payload.Token.CrossGroupRetry = authority.Token.ModelLimitsEnabled, authority.Token.CrossGroupRetry
	payload.Options, payload.Abilities = permissionOptions, abilities
	encoded, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// canvasManagedTokenContractValid 校验不会随余额消耗变化的管理 Token 固定字段。
func canvasManagedTokenContractValid(managed *CanvasManagedToken, token *Token) bool {
	if managed == nil || token == nil || token.Id != managed.TokenID || token.UserId != managed.UserID ||
		token.Group != managed.GroupID || token.CrossGroupRetry || !token.UnlimitedQuota ||
		token.ModelLimitsEnabled || token.ModelLimits != "" {
		return false
	}
	if token.AllowIps != nil && strings.TrimSpace(*token.AllowIps) != "" {
		return false
	}
	if managed.KeyFingerprint != "" {
		fingerprint := sha256.Sum256([]byte(token.GetFullKey()))
		if hex.EncodeToString(fingerprint[:]) != managed.KeyFingerprint {
			return false
		}
	}
	groups, err := token.GetAutoGroups()
	if err != nil {
		return false
	}
	managedGroups, err := canvasManagedAutoGroups(managed)
	if err != nil {
		return false
	}
	if !equalStringSlices(groups, managedGroups) {
		return false
	}
	if managed.GroupID == "auto" {
		return len(groups) > 0 && !slicesContainsString(groups, CanvasExcludedGroup)
	}
	return len(groups) == 0
}

// canvasManagedTokenContractValidIgnoringGroup 校验除分组字段外的管理 Token 固定合同。
// 该判别只用于显式 repair，避免把禁用、过期或权限变化误判成可修复的人工改组。
func canvasManagedTokenContractValidIgnoringGroup(managed *CanvasManagedToken, token *Token) bool {
	if managed == nil || token == nil || token.Id != managed.TokenID || token.UserId != managed.UserID ||
		token.CrossGroupRetry || !token.UnlimitedQuota || token.ModelLimitsEnabled || token.ModelLimits != "" {
		return false
	}
	if token.AllowIps != nil && strings.TrimSpace(*token.AllowIps) != "" {
		return false
	}
	if managed.KeyFingerprint != "" {
		fingerprint := sha256.Sum256([]byte(token.GetFullKey()))
		if hex.EncodeToString(fingerprint[:]) != managed.KeyFingerprint {
			return false
		}
	}
	groups, err := token.GetAutoGroups()
	if err != nil {
		return false
	}
	if managed.GroupID == "auto" || token.Group == "auto" {
		// 分组被人工改动时后台可能同时清空或重写 auto_groups；repair
		// 会以当前调用方过滤后的范围覆盖该字段，因此这里只需确认 JSON 可解析。
		return true
	}
	managedGroups, err := canvasManagedAutoGroups(managed)
	if err != nil || !equalStringSlices(groups, managedGroups) {
		return false
	}
	return len(groups) == 0
}

// canvasPermissionOptionsWithTx 从权威配置表提取权限语义，并排除余额及价格数值。
func canvasPermissionOptionsWithTx(tx *gorm.DB, authority *CanvasManagedAuthority) (canvasPermissionOptions, error) {
	keys := []string{
		"UserUsableGroups", "GroupRatio", "MaxTokenAutoGroups",
		"group_ratio_setting.group_ratio", "group_ratio_setting.group_special_usable_group",
	}
	var options []Option
	if err := tx.Where(commonKeyCol+" IN ?", keys).Order(commonKeyCol + " ASC").Find(&options).Error; err != nil {
		return canvasPermissionOptions{}, err
	}
	snapshot := canvasPermissionOptions{}
	for _, option := range options {
		switch option.Key {
		case "UserUsableGroups":
			var values map[string]string
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return canvasPermissionOptions{}, err
			}
			snapshot.UserUsableGroups = sortedCanvasMapKeys(values)
		case "GroupRatio":
			var values map[string]float64
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return canvasPermissionOptions{}, err
			}
			snapshot.LegacyRatioGroups = sortedCanvasMapKeys(values)
		case "group_ratio_setting.group_ratio":
			var values map[string]float64
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return canvasPermissionOptions{}, err
			}
			snapshot.ConfiguredRatioGroups = sortedCanvasMapKeys(values)
		case "group_ratio_setting.group_special_usable_group":
			var values map[string]map[string]string
			if err := common.UnmarshalJsonStr(option.Value, &values); err != nil {
				return canvasPermissionOptions{}, err
			}
			snapshot.SpecialGroupDirectives = sortedCanvasMapKeys(values[authority.User.Group])
		case "MaxTokenAutoGroups":
			if authority.Managed.GroupID == "auto" {
				snapshot.MaxTokenAutoGroups = strings.TrimSpace(option.Value)
			}
		}
	}
	return snapshot, nil
}

// sortedCanvasMapKeys 返回稳定排序的配置键，用于权限指纹序列化。
func sortedCanvasMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// canvasOperationDigest 将客户端操作绑定到固定 grant 和原始分组。
func canvasOperationDigest(grantID, group string) string {
	encoded, _ := common.Marshal(struct {
		GrantID string `json:"grant_id"`
		Group   string `json:"group"`
	}{GrantID: grantID, Group: group})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// canvasRepairOperationDigest 将显式 repair 的身份事实和范围绑定到独立摘要空间。
// operation_id 由唯一索引绑定；摘要仍包含它，避免不同 repair 意图共享请求事实。
func canvasRepairOperationDigest(input CanvasManagedTokenRepairInput) string {
	encoded, _ := common.Marshal(struct {
		GrantID     string                   `json:"grant_id"`
		GroupID     string                   `json:"group_id"`
		OperationID string                   `json:"operation_id"`
		AutoGroups  []string                 `json:"auto_groups,omitempty"`
		Repair      CanvasManagedTokenRepair `json:"repair"`
	}{
		GrantID: input.GrantID, GroupID: input.GroupID, OperationID: input.OperationID,
		AutoGroups: input.AutoGroups, Repair: input.Repair,
	})
	digest := sha256.Sum256([]byte("repair:" + string(encoded)))
	return hex.EncodeToString(digest[:])
}

// canvasManagedTokenName 生成不超过 50 个 UTF-8 字节且带稳定防碰撞后缀的名称。
func canvasManagedTokenName(instanceID, group string) string {
	readable := strings.TrimSpace(instanceID + "-" + group)
	digest := sha256.Sum256([]byte(instanceID + "\x00" + group))
	suffix := hex.EncodeToString(digest[:5])
	const prefix = "Canvas-"
	limit := 50 - len(prefix) - 1 - len(suffix)
	readable = truncateUTF8Bytes(readable, limit)
	return prefix + readable + "-" + suffix
}

// truncateUTF8Bytes 在不截断 UTF-8 编码单元的前提下限制字节长度。
func truncateUTF8Bytes(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for len(value) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return value
}

// normalizeCanvasAutoGroups 保序去重、去空白并精确排除禁用分组。
func normalizeCanvasAutoGroups(groups []string) []string {
	result := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || group == CanvasExcludedGroup {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return result
}

// canvasManagedAutoGroups 解析管理关系冻结的显式 Auto 范围。
func canvasManagedAutoGroups(managed *CanvasManagedToken) ([]string, error) {
	if managed == nil || managed.AutoGroups == "" {
		return nil, nil
	}
	var groups []string
	if err := common.UnmarshalJsonStr(managed.AutoGroups, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// equalStringSlices 按顺序比较两个字符串切片。
func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// slicesContainsString 判断切片是否精确包含目标字符串。
func slicesContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
