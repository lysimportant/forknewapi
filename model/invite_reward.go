package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

// 邀请奖励按笔冻结提现的账本实现。
//
// 汇总字段 users.aff_quota 仍是对外展示的“可提现邀请额度”，但不再作为唯一凭据：
// 每一笔奖励在 invite_rewards 中独立记录产生时间和剩余额度，48 小时冻结期按笔
// 从自身产生时间起算。提现时按到期时间、记录 ID 升序逐笔扣减，汇总与账本在同一个
// 事务内更新，避免任何一方单独漂移。
const (
	// InviteRewardKindInviter 邀请人奖励：写入账本并按笔冻结。
	InviteRewardKindInviter = "inviter"
	// InviteRewardKindInvitee 受邀人注册赠送：直接计入站内余额，不进入冻结账本。
	InviteRewardKindInvitee = "invitee"
	// InviteRewardKindHistory 迁移前历史余额结转：属于明确的历史例外，产生即可提现。
	InviteRewardKindHistory = "history"

	// 账本状态。
	inviteRewardStatusFrozen    = "frozen"
	inviteRewardStatusAvailable = "available"
	inviteRewardStatusPartial   = "partial"
	inviteRewardStatusWithdrawn = "withdrawn"

	inviteRewardWithdrawalPageSize = 50

	// inviteRewardListLimit 限制汇总响应携带的明细条数，完整账本通过分页接口读取。
	inviteRewardListLimit = 50
)

var (
	// ErrInviteRewardIdempotencyKeyMissing 表示写请求缺少幂等标识，旧客户端必须升级。
	ErrInviteRewardIdempotencyKeyMissing = errors.New("missing idempotency key")
	// ErrInviteRewardQuotaNotPositive 表示请求金额不是正数。
	ErrInviteRewardQuotaNotPositive = errors.New("invite reward quota must be positive")
	// ErrInviteRewardQuotaInsufficient 表示可提现金额不足（含全部处于冻结期的情况）。
	ErrInviteRewardQuotaInsufficient = errors.New("insufficient withdrawable invite reward")
	// ErrInviteRewardConcurrentModification 表示账本在事务内被并发修改，本次提现整体回滚。
	ErrInviteRewardConcurrentModification = errors.New("invite reward ledger changed concurrently")
)

// InviteRewardFrozenError 携带最近一笔奖励的可提现时间，便于前端直接展示倒计时。
type InviteRewardFrozenError struct {
	NextAvailableAt int64
}

func (e *InviteRewardFrozenError) Error() string {
	return fmt.Sprintf("invite reward is still frozen until %d", e.NextAvailableAt)
}

// InviteReward 是邀请奖励账本的一笔记录。
//
// SourceKey 是去重依据：邀请人奖励为 "inviter:<受邀用户ID>"，受邀人赠送为
// "invitee:<受邀用户ID>"，历史结转为 "history:<用户ID>"。它同时承担幂等键职责，
// 因此重复注册回调、重复启动迁移都不会产生第二笔奖励。
type InviteReward struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	UserId         int    `json:"user_id" gorm:"column:user_id;type:bigint;not null;uniqueIndex:uk_invite_rewards_source,priority:1;index:idx_invite_rewards_user_available,priority:1"`
	SourceKey      string `json:"source_key" gorm:"column:source_key;type:varchar(64);not null;uniqueIndex:uk_invite_rewards_source,priority:2"`
	Kind           string `json:"kind" gorm:"type:varchar(16);not null"`
	Quota          int    `json:"quota" gorm:"type:bigint;not null"`
	RemainingQuota int    `json:"remaining_quota" gorm:"column:remaining_quota;type:bigint;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_invite_rewards_user_available,priority:2"`
	// AvailableAt 为 0 表示按 CreatedAt 加冻结时长计算；历史结转显式写 0 且 Kind 为
	// history，因此不受冻结期约束。
	AvailableAt int64 `json:"available_at" gorm:"column:available_at;type:bigint;not null;default:0"`
}

// TableName 固定表名，避免 GORM 复数化规则随模型改名而变。
func (InviteReward) TableName() string {
	return "invite_rewards"
}

// WithdrawableAfter 返回该笔奖励最早可提现的服务端时间戳，单位为秒。
// 冻结时长按当前配置计算（管理员可调整，0 表示取消冻结期），因此配置变更立即生效。
func (reward *InviteReward) WithdrawableAfter() int64 {
	if reward.Kind == InviteRewardKindHistory {
		return reward.AvailableAt
	}
	if reward.AvailableAt > 0 {
		return reward.AvailableAt
	}
	return reward.CreatedAt + int64(inviteRewardFreezeWindow())
}

// InviteRewardWithdrawal 记录一次提现请求及其账本分配。
//
// IdempotencyKey 由客户端提供，与用户 ID 组成唯一索引；重复提交（连点、网络重试、
// 多会话并发）只能落库一次，其余请求返回首次结果。
type InviteRewardWithdrawal struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	UserId         int    `json:"user_id" gorm:"column:user_id;type:bigint;not null;uniqueIndex:uk_invite_reward_withdrawals_request,priority:1;index:idx_invite_reward_withdrawals_user_created,priority:1"`
	IdempotencyKey string `json:"-" gorm:"column:idempotency_key;type:varchar(128);not null;uniqueIndex:uk_invite_reward_withdrawals_request,priority:2"`
	Quota          int    `json:"quota" gorm:"type:bigint;not null"`
	// Allocations 是本次提现扣减的账本明细 JSON，用于对账与旧版兼容。
	Allocations string `json:"allocations" gorm:"type:text"`
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_invite_reward_withdrawals_user_created,priority:2"`
}

// TableName 固定表名，避免 GORM 复数化规则随模型改名而变。
func (InviteRewardWithdrawal) TableName() string {
	return "invite_reward_withdrawals"
}

// InviteRewardAllocation 是提现分配明细中的一项。
type InviteRewardAllocation struct {
	RewardId int `json:"reward_id"`
	Quota    int `json:"quota"`
}

// InviteRewardItem 是对外返回的账本明细，附带当前状态与可提现时间。
type InviteRewardItem struct {
	Id              int    `json:"id"`
	Kind            string `json:"kind"`
	Quota           int    `json:"quota"`
	RemainingQuota  int    `json:"remaining_quota"`
	Status          string `json:"status"`
	CreatedAt       int64  `json:"created_at"`
	NextAvailableAt int64  `json:"next_available_at"`
}

// InviteRewardSummary 是当前用户的奖励汇总。
type InviteRewardSummary struct {
	AffQuota          int                      `json:"aff_quota"`
	AffHistoryQuota   int                      `json:"aff_history_quota"`
	AffCount          int                      `json:"aff_count"`
	FrozenQuota       int                      `json:"frozen_quota"`
	WithdrawableQuota int                      `json:"withdrawable_quota"`
	WithdrawnQuota    int                      `json:"withdrawn_quota"`
	NextAvailableAt   int64                    `json:"next_available_at"`
	MinimumQuota      int                      `json:"minimum_quota"`
	FreezeSeconds     int                      `json:"freeze_seconds"`
	ServerTime        int64                    `json:"server_time"`
	Rewards           []InviteRewardItem       `json:"rewards"`
	Withdrawals       []InviteRewardWithdrawal `json:"withdrawals"`
}

// inviteRewardFreezeWindow 返回当前生效的冻结时长（秒）。
// 管理员可通过 InviteRewardFreezeHours 调整；0 表示取消冻结期，负值按 0 处理。
func inviteRewardFreezeWindow() int {
	if common.InviteRewardFreezeHours <= 0 {
		return 0
	}
	return common.InviteRewardFreezeHours * 60 * 60
}

// inviteRewardStatusFor 依据剩余额度和可提现时间推导展示状态。
func inviteRewardStatusFor(remaining int, withdrawableAfter, now int64) string {
	if remaining <= 0 {
		return inviteRewardStatusWithdrawn
	}
	if now < withdrawableAfter {
		return inviteRewardStatusFrozen
	}
	return inviteRewardStatusAvailable
}

// GrantInviteReward 在事务内为邀请人写入一笔冻结奖励，并同步邀请计数与汇总余额。
//
// 返回 (false, nil) 表示该来源已发放过（重复回调、重复注册流程），调用方无需处理，
// 也绝不能再次记账。奖励写入、邀请计数与汇总余额在同一事务内完成，失败时整笔回滚。
func GrantInviteReward(tx *gorm.DB, inviterId int, sourceKey, kind string, quota int, createdAt int64) (bool, error) {
	if quota <= 0 {
		return false, nil
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return false, err
	}

	claimed, err := ClaimInviteReward(tx, inviterId, sourceKey, kind, quota, createdAt)
	if err != nil || !claimed {
		return claimed, err
	}

	result := tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]interface{}{
		"aff_count":   gorm.Expr("aff_count + ?", 1),
		"aff_quota":   gorm.Expr("aff_quota + ?", quota),
		"aff_history": gorm.Expr("aff_history + ?", quota),
	})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, gorm.ErrRecordNotFound
	}
	return true, nil
}

// ClaimInviteReward 只负责占用奖励来源：已存在时返回 (false, nil)。
// 余额、邀请计数等副作用由调用方在确认 claimed 为 true 后执行，保证重复调用不重复计款。
func ClaimInviteReward(tx *gorm.DB, userId int, sourceKey, kind string, quota int, createdAt int64) (bool, error) {
	var existing int64
	if err := tx.Model(&InviteReward{}).
		Where("user_id = ? AND source_key = ?", userId, sourceKey).
		Count(&existing).Error; err != nil {
		return false, err
	}
	if existing > 0 {
		return false, nil
	}
	reward := InviteReward{
		UserId:         userId,
		SourceKey:      sourceKey,
		Kind:           kind,
		Quota:          quota,
		RemainingQuota: quota,
		CreatedAt:      createdAt,
	}
	if err := tx.Create(&reward).Error; err != nil {
		return false, err
	}
	return true, nil
}

// WithdrawInviteRewardsAt 是提现的核心实现，now 为服务端当前时间戳（秒）。
// 校验归属与到期状态，按到期时间、记录 ID 升序扣减账本，然后增加站内余额并写入
// 提现记录；显式传入 now 便于测试覆盖 47:59:59 与 48:00:00 边界。
func WithdrawInviteRewardsAt(userId int, requestId string, requested int, now int64) (int, error) {
	if requested <= 0 {
		return 0, ErrInviteRewardQuotaNotPositive
	}
	if err := common.ValidateWalletQuota(requested); err != nil {
		return 0, err
	}
	if float64(requested) < common.QuotaPerUnit {
		return 0, fmt.Errorf("转移额度最小为%s！", logger.LogQuota(common.QuotaFromFloat(common.QuotaPerUnit)))
	}

	var (
		withdrawal InviteRewardWithdrawal
		user       User
	)
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&user, userId).Error; err != nil {
			return err
		}
		if user.AffQuota < requested {
			return ErrInviteRewardQuotaInsufficient
		}

		// 先占用幂等键：唯一索引让重复提交在这里失败，避免同一次提现被记两次。
		withdrawal = InviteRewardWithdrawal{
			UserId:         userId,
			IdempotencyKey: requestId,
			Quota:          requested,
			CreatedAt:      now,
		}
		if err := tx.Create(&withdrawal).Error; err != nil {
			return err
		}

		var candidates []InviteReward
		if err := tx.Where("user_id = ? AND remaining_quota > 0", userId).
			Order("created_at ASC, id ASC").Find(&candidates).Error; err != nil {
			return err
		}

		allocations := make([]InviteRewardAllocation, 0, len(candidates))
		deducted := 0
		var earliestFrozen int64
		for i := range candidates {
			reward := candidates[i]
			withdrawableAfter := reward.WithdrawableAfter()
			if now < withdrawableAfter {
				if earliestFrozen == 0 || withdrawableAfter < earliestFrozen {
					earliestFrozen = withdrawableAfter
				}
				continue
			}
			remaining := requested - deducted
			if remaining <= 0 {
				break
			}
			take := min(reward.RemainingQuota, remaining)
			// 条件更新同时充当并发保护：扣减只会发生在剩余额度仍然足够时，
			// 受影响行数不为 1 说明账本已被并发修改，整笔事务回滚。
			result := tx.Model(&InviteReward{}).
				Where("id = ? AND remaining_quota >= ?", reward.Id, take).
				Update("remaining_quota", gorm.Expr("remaining_quota - ?", take))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrInviteRewardConcurrentModification
			}
			allocations = append(allocations, InviteRewardAllocation{RewardId: reward.Id, Quota: take})
			deducted += take
		}

		if deducted != requested {
			return inviteRewardShortfall(deducted, earliestFrozen)
		}

		result := tx.Model(&User{}).
			Where("id = ? AND quota <= ?", userId, common.MaxWalletQuota-requested).
			Updates(map[string]interface{}{
				"aff_quota": gorm.Expr("aff_quota - ?", requested),
				"quota":     gorm.Expr("quota + ?", requested),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var count int64
			if err := tx.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return gorm.ErrRecordNotFound
			}
			return ErrWalletQuotaLimitExceeded
		}

		payload, err := common.Marshal(allocations)
		if err != nil {
			return err
		}
		return tx.Model(&InviteRewardWithdrawal{}).
			Where("id = ?", withdrawal.Id).
			Update("allocations", string(payload)).Error
	})
	if err != nil {
		return 0, err
	}

	syncCreditUserQuotaCache(userId, requested, "invite reward withdrawal")
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("提取邀请奖励至站内余额 %s", logger.LogQuota(requested)))
	return requested, nil
}

// inviteRewardShortfall 区分“金额不足”和“金额已到期但被冻结”两种情况，
// 后者返回最近一笔的可提现时间，避免把冻结误报为余额不足。
func inviteRewardShortfall(deducted int, earliestFrozen int64) error {
	if deducted == 0 && earliestFrozen > 0 {
		return &InviteRewardFrozenError{NextAvailableAt: earliestFrozen}
	}
	return ErrInviteRewardQuotaInsufficient
}

// WithdrawInviteRewards 执行一次幂等提现。重复的 requestId 直接返回首次结果，
// 不会重复记账；缺少幂等标识的请求被拒绝，旧客户端必须升级后才能提现。
func WithdrawInviteRewards(userId int, requestId string, requested int) (int, error) {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return 0, ErrInviteRewardIdempotencyKeyMissing
	}
	if len(requestId) > 128 {
		return 0, ErrInviteRewardIdempotencyKeyMissing
	}
	if userId <= 0 {
		return 0, gorm.ErrRecordNotFound
	}
	quota, err := WithdrawInviteRewardsAt(userId, requestId, requested, common.GetTimestamp())
	if err == nil {
		return quota, nil
	}

	// 唯一索引冲突说明同一 requestId 已经成功入账，返回首次结果而不是报错。
	var existing InviteRewardWithdrawal
	if findErr := DB.Where("user_id = ? AND idempotency_key = ?", userId, requestId).First(&existing).Error; findErr == nil {
		return existing.Quota, nil
	}
	return 0, err
}

// GetInviteRewardSummary 汇总当前用户的奖励状态，供钱包页面展示冻结与可提金额。
func GetInviteRewardSummary(userId int) (*InviteRewardSummary, error) {
	user, err := GetUserById(userId, false)
	if err != nil {
		return nil, err
	}

	var rewards []InviteReward
	if err := DB.Where("user_id = ?", userId).
		Order("created_at DESC, id DESC").Limit(inviteRewardListLimit).Find(&rewards).Error; err != nil {
		return nil, err
	}

	var withdrawals []InviteRewardWithdrawal
	if err := DB.Where("user_id = ?", userId).
		Order("created_at DESC, id DESC").Limit(inviteRewardWithdrawalPageSize).Find(&withdrawals).Error; err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	var frozen, withdrawn int
	var nextAvailableAt int64
	items := make([]InviteRewardItem, 0, len(rewards))
	for _, reward := range rewards {
		withdrawableAfter := reward.WithdrawableAfter()
		if reward.RemainingQuota > 0 && now < withdrawableAfter {
			frozen += reward.RemainingQuota
			if nextAvailableAt == 0 || withdrawableAfter < nextAvailableAt {
				nextAvailableAt = withdrawableAfter
			}
		}
		withdrawn += reward.Quota - reward.RemainingQuota
		items = append(items, InviteRewardItem{
			Id:              reward.Id,
			Kind:            reward.Kind,
			Quota:           reward.Quota,
			RemainingQuota:  reward.RemainingQuota,
			Status:          inviteRewardStatusFor(reward.RemainingQuota, withdrawableAfter, now),
			CreatedAt:       reward.CreatedAt,
			NextAvailableAt: withdrawableAfter,
		})
	}

	return &InviteRewardSummary{
		AffQuota:          user.AffQuota,
		AffHistoryQuota:   user.AffHistoryQuota,
		AffCount:          user.AffCount,
		FrozenQuota:       frozen,
		WithdrawableQuota: user.AffQuota - frozen,
		WithdrawnQuota:    withdrawn,
		NextAvailableAt:   nextAvailableAt,
		MinimumQuota:      common.QuotaFromFloat(common.QuotaPerUnit),
		FreezeSeconds:     inviteRewardFreezeWindow(),
		ServerTime:        now,
		Rewards:           items,
		Withdrawals:       withdrawals,
	}, nil
}

// InitializeInviteRewardLedger 把账本引入之前的汇总余额结转为一笔历史记录。
//
// 历史余额无法可靠还原每笔产生时间，因此按明确的历史例外处理：结转记录产生即可
// 提现。SourceKey 固定为 "history:<用户ID>"，重复启动不会产生第二笔结转。
func InitializeInviteRewardLedger() error {
	var users []User
	if err := DB.Select("id", "aff_quota").Where("aff_quota > 0").Order("id ASC").Find(&users).Error; err != nil {
		return err
	}
	for _, user := range users {
		sourceKey := fmt.Sprintf("%s:%d", InviteRewardKindHistory, user.Id)
		var existing InviteReward
		err := DB.Where("user_id = ? AND source_key = ?", user.Id, sourceKey).First(&existing).Error
		if err == nil {
			// 已结转：余额只能由提现减少，出现增长说明汇总与账本不一致，显式记录而不猜测。
			if existing.RemainingQuota != user.AffQuota {
				common.SysError(fmt.Sprintf("invite reward ledger mismatch for user %d: ledger=%d summary=%d",
					user.Id, existing.RemainingQuota, user.AffQuota))
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		carryover := InviteReward{
			UserId:         user.Id,
			SourceKey:      sourceKey,
			Kind:           InviteRewardKindHistory,
			Quota:          user.AffQuota,
			RemainingQuota: user.AffQuota,
			CreatedAt:      common.GetTimestamp(),
		}
		if err := DB.Create(&carryover).Error; err != nil {
			return err
		}
	}
	return nil
}

// inviteRewardSourceKey 生成邀请奖励的去重标识。
func inviteRewardSourceKey(kind string, inviteeId int) string {
	return fmt.Sprintf("%s:%d", kind, inviteeId)
}
