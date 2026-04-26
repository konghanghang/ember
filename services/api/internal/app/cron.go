package app

import (
	"context"
	"log"
	"strconv"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/db"
	"github.com/konghang/ember/backend/internal/models"
	accountpkg "github.com/konghang/ember/backend/internal/services/account"
	devicepkg "github.com/konghang/ember/backend/internal/services/device"
	emailpkg "github.com/konghang/ember/backend/internal/services/email"
	mediagappkg "github.com/konghang/ember/backend/internal/services/mediagap"
	paymentpkg "github.com/konghang/ember/backend/internal/services/payment"
	playbackpkg "github.com/konghang/ember/backend/internal/services/playback"
	systempkg "github.com/konghang/ember/backend/internal/services/system"
	telegrampkg "github.com/konghang/ember/backend/internal/services/telegram"
	tvcalendarpkg "github.com/konghang/ember/backend/internal/services/tvcalendar"
	"github.com/robfig/cron/v3"
)

func initCronJobs() func() {
	configService := configpkg.NewConfigService()

	cronEnabledStr := configService.GetString("CRON_ENABLED")
	if cronEnabledStr == "" {
		cronEnabledStr = "true"
	}
	cronEnabled, err := strconv.ParseBool(cronEnabledStr)
	if err != nil {
		log.Printf("[Cron] CRON_ENABLED 配置无效（%q），回退为 true：%v", cronEnabledStr, err)
		cronEnabled = true
	}
	if !cronEnabled {
		return func() {}
	}

	expiredSchedule := configService.GetString("CRON_SCHEDULE")
	if expiredSchedule == "" {
		expiredSchedule = "0 2 * * *"
	}

	rankingCronEnabledStr := configService.GetString("RANKING_CRON_ENABLED")
	if rankingCronEnabledStr == "" {
		rankingCronEnabledStr = "false"
	}
	rankingCronEnabled, err := strconv.ParseBool(rankingCronEnabledStr)
	if err != nil {
		log.Printf("[Cron] RANKING_CRON_ENABLED 配置无效（%q），回退为 false：%v", rankingCronEnabledStr, err)
		rankingCronEnabled = false
	}

	rankingDailySchedule := configService.GetString("RANKING_DAILY_SCHEDULE")
	if rankingDailySchedule == "" {
		rankingDailySchedule = "0 20 * * *"
	}

	rankingWeeklySchedule := configService.GetString("RANKING_WEEKLY_SCHEDULE")
	if rankingWeeklySchedule == "" {
		rankingWeeklySchedule = "30 20 * * 0"
	}

	tvCalendarSyncSchedule := configService.GetString("TV_CALENDAR_SYNC_SCHEDULE")
	if tvCalendarSyncSchedule == "" {
		tvCalendarSyncSchedule = "0 */12 * * *"
	}

	tvCalendarStartupSyncEnabledStr := configService.GetString("TV_CALENDAR_STARTUP_SYNC_ENABLED")
	if tvCalendarStartupSyncEnabledStr == "" {
		tvCalendarStartupSyncEnabledStr = "true"
	}
	tvCalendarStartupSyncEnabled, err := strconv.ParseBool(tvCalendarStartupSyncEnabledStr)
	if err != nil {
		log.Printf("[Cron] TV_CALENDAR_STARTUP_SYNC_ENABLED 配置无效（%q），回退为 true：%v", tvCalendarStartupSyncEnabledStr, err)
		tvCalendarStartupSyncEnabled = true
	}

	tzName := configService.GetString("CRON_TIMEZONE")
	if tzName == "" {
		tzName = "Asia/Shanghai"
	}

	tz, err := time.LoadLocation(tzName)
	if err != nil {
		log.Printf("时区解析失败，使用 UTC：%v", err)
		tz = time.UTC
	}

	c := cron.New(cron.WithLocation(tz))

	systemService := systempkg.NewSystemService()
	emailService := emailpkg.NewEmailService()
	telegramService := telegrampkg.NewDefaultService()
	tvCalendarService := tvcalendarpkg.NewTVCalendarService()
	embyCompensation := accountpkg.NewEmbyCompensation(nil)
	mediaGapScanRecorder := mediagappkg.NewMediaGapScanRecorder()
	var rankingService *playbackpkg.PlaybackRankingService
	if rankingCronEnabled {
		rankingService = playbackpkg.NewPlaybackRankingService()
	}

	taskRegistered := false

	if _, err := c.AddFunc("0 3 * * *", func() {
		count, err := emailService.CleanupExpired()
		if err != nil {
			log.Printf("[Cron] 清理过期验证码失败：%v", err)
		} else if count > 0 {
			log.Printf("[Cron] 已清理 %d 条过期验证码", count)
		}

		telegramCount, telegramErr := telegramService.CleanupExpiredBindCodes()
		if telegramErr != nil {
			log.Printf("[Cron] 清理过期 Telegram 绑定码失败：%v", telegramErr)
		} else if telegramCount > 0 {
			log.Printf("[Cron] 已清理 %d 条过期 Telegram 绑定码", telegramCount)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（验证码清理）：%v", err)
	} else {
		taskRegistered = true
	}

	if _, err := c.AddFunc(expiredSchedule, func() {
		log.Println("[Cron] 开始检查过期用户...")
		result, err := systemService.CheckExpiredUsers()
		if err != nil {
			log.Printf("[Cron] 检查失败：%v", err)
			return
		}
		log.Printf("[Cron] 完成，封禁 %d/%d 个用户", result.DisabledCount, result.TotalExpired)
	}); err != nil {
		log.Printf("定时任务注册失败（过期检查）：%v", err)
	} else {
		taskRegistered = true
	}

	if _, err := c.AddFunc("@every 10m", func() {
		result, err := embyCompensation.Process(context.Background())
		if err != nil {
			log.Printf("[Cron] Emby 补偿队列处理失败：%v", err)
			return
		}
		if result.Processed > 0 || result.Failed > 0 {
			log.Printf("[Cron] Emby 补偿队列：成功 %d 条，失败 %d 条", result.Processed, result.Failed)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（Emby 补偿队列）：%v", err)
	} else {
		taskRegistered = true
	}

	if _, err := c.AddFunc("@weekly", func() {
		removed, err := mediaGapScanRecorder.CleanupOldScans(context.Background())
		if err != nil {
			log.Printf("[Cron] 清理 media_gap_scans 历史记录失败：%v", err)
			return
		}
		if removed > 0 {
			log.Printf("[Cron] 清理 media_gap_scans 历史记录 %d 条", removed)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（media_gap_scans 清理）：%v", err)
	} else {
		taskRegistered = true
	}

	if rankingCronEnabled {
		if _, err := c.AddFunc(rankingDailySchedule, func() {
			log.Println("[Cron] 开始生成播放日榜...")
			if err := rankingService.GenerateRanking(models.RankingDaily, nil, nil); err != nil {
				log.Printf("[Cron] 日榜生成失败：%v", err)
				return
			}
			log.Println("[Cron] 日榜生成完成")
		}); err != nil {
			log.Printf("定时任务注册失败（日榜）：%v", err)
		} else {
			taskRegistered = true
		}

		if _, err := c.AddFunc(rankingWeeklySchedule, func() {
			log.Println("[Cron] 开始生成播放周榜...")
			if err := rankingService.GenerateRanking(models.RankingWeekly, nil, nil); err != nil {
				log.Printf("[Cron] 周榜生成失败：%v", err)
				return
			}
			log.Println("[Cron] 周榜生成完成")
		}); err != nil {
			log.Printf("定时任务注册失败（周榜）：%v", err)
		} else {
			taskRegistered = true
		}
	}

	if tvCalendarService.SyncAvailable() {
		if tvCalendarStartupSyncEnabled {
			time.AfterFunc(15*time.Second, func() {
				count, err := tvCalendarService.SyncCalendar(context.Background(), tvcalendarpkg.DefaultTVCalendarWeekOffsets(), nil, false)
				if err != nil {
					log.Printf("[TV Calendar] 启动补偿同步失败：%v", err)
					return
				}
				log.Printf("[TV Calendar] 启动补偿同步完成，处理 %d 条记录", count)
			})
		} else {
			log.Printf("[TV Calendar] 启动补偿同步已禁用")
		}

		if _, err := c.AddFunc(tvCalendarSyncSchedule, func() {
			log.Println("[Cron] 开始同步追剧日历...")
			count, err := tvCalendarService.SyncCalendar(context.Background(), tvcalendarpkg.DefaultTVCalendarWeekOffsets(), nil, false)
			if err != nil {
				log.Printf("[Cron] 追剧日历同步失败：%v", err)
				return
			}
			log.Printf("[Cron] 追剧日历同步完成，处理 %d 条记录", count)
		}); err != nil {
			log.Printf("定时任务注册失败（追剧日历同步）：%v", err)
		} else {
			taskRegistered = true
		}
	} else {
		log.Printf("追剧日历自动同步未启用：缺少 Emby 或 TMDB 配置")
	}

	if !taskRegistered {
		return func() {}
	}

	// device-actions-cleanup（每日凌晨 5 点，保留 90 天）
	if _, err := c.AddFunc("0 5 * * *", func() {
		removed, err := devicepkg.CleanupOldDeviceActions(context.Background(), 90)
		if err != nil {
			log.Printf("[Cron] device-actions-cleanup 失败：%v", err)
		} else if removed > 0 {
			log.Printf("[Cron] device-actions-cleanup：清理 %d 条", removed)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（device-actions-cleanup）：%v", err)
	} else {
		taskRegistered = true
	}

	// playback-rankings-cleanup（每周日凌晨 5 点，保留 90 天）
	if _, err := c.AddFunc("0 5 * * 0", func() {
		removed, err := playbackpkg.CleanupOldRankings(context.Background(), 90)
		if err != nil {
			log.Printf("[Cron] playback-rankings-cleanup 失败：%v", err)
		} else if removed > 0 {
			log.Printf("[Cron] playback-rankings-cleanup：清理 %d 条", removed)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（playback-rankings-cleanup）：%v", err)
	} else {
		taskRegistered = true
	}

	// payments-expire-pending（每 5 分钟推进 pending → expired）
	if _, err := c.AddFunc("@every 5m", func() {
		expired, err := paymentpkg.ExpirePendingPayments(context.Background())
		if err != nil {
			log.Printf("[Cron] payments-expire-pending 失败：%v", err)
		} else if expired > 0 {
			log.Printf("[Cron] payments-expire-pending：推进 %d 条 pending → expired", expired)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（payments-expire-pending）：%v", err)
	} else {
		taskRegistered = true
	}

	// media-quality-inflight-sweep（每 30 分钟清理残留的 inflight 标记）
	if _, err := c.AddFunc("@every 30m", func() {
		result := db.DB.Exec(`UPDATE media_quality_caches SET "inflightUntil" = NULL WHERE "inflightUntil" < now() - interval '1 hour'`)
		if result.Error != nil {
			log.Printf("[Cron] media-quality-inflight-sweep 失败：%v", result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("[Cron] media-quality-inflight-sweep：清理 %d 条残留 inflight", result.RowsAffected)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（media-quality-inflight-sweep）：%v", err)
	}

	// tmdb-cache-gc（每日凌晨 4 点，删除超期 7 天的过期缓存）
	// tmdb-cache-gc（每日凌晨 4 点，删除超期 7 天的过期缓存）
	if _, err := c.AddFunc("0 4 * * *", func() {
		removed, err := tvcalendarpkg.RunTMDBCacheGC(context.Background())
		if err != nil {
			log.Printf("[Cron] tmdb-cache-gc 失败：%v", err)
		} else if removed > 0 {
			log.Printf("[Cron] tmdb-cache-gc：删除 %d 条过期缓存", removed)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（tmdb-cache-gc）：%v", err)
	}

	// bot-pending-reject-cleanup（每 30 分钟清理过期的拒绝待确认记录）
	if _, err := c.AddFunc("@every 30m", func() {
		removed, err := telegrampkg.CleanupExpiredPendingRejects(context.Background())
		if err != nil {
			log.Printf("[Cron] bot-pending-reject-cleanup 失败：%v", err)
		} else if removed > 0 {
			log.Printf("[Cron] bot-pending-reject-cleanup：清理 %d 条过期记录", removed)
		}
	}); err != nil {
		log.Printf("定时任务注册失败（bot-pending-reject-cleanup）：%v", err)
	}

	c.Start()
	if rankingCronEnabled {
		log.Printf(
			"定时任务已启用：过期检查(%s), 日榜(%s), 周榜(%s), 追剧日历(%s) (%s)",
			expiredSchedule,
			rankingDailySchedule,
			rankingWeeklySchedule,
			tvCalendarSyncSchedule,
			tzName,
		)
	} else {
		log.Printf("定时任务已启用：过期检查(%s), 追剧日历(%s) (%s)", expiredSchedule, tvCalendarSyncSchedule, tzName)
	}

	return func() {
		ctx := c.Stop()
		<-ctx.Done()
	}
}
