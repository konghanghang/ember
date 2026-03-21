package app

import (
	"context"
	"log"
	"time"

	configpkg "github.com/konghang/ember/backend/internal/config"
	"github.com/konghang/ember/backend/internal/models"
	"github.com/konghang/ember/backend/internal/services"
	"github.com/robfig/cron/v3"
)

func initCronJobs() func() {
	configService := configpkg.NewConfigService()

	cronEnabled := configService.GetString("CRON_ENABLED")
	if cronEnabled == "" {
		cronEnabled = "true"
	}
	if cronEnabled != "true" {
		return func() {}
	}

	expiredSchedule := configService.GetString("CRON_SCHEDULE")
	if expiredSchedule == "" {
		expiredSchedule = "0 2 * * *"
	}

	rankingCronEnabled := configService.GetString("RANKING_CRON_ENABLED")
	if rankingCronEnabled == "" {
		rankingCronEnabled = "false"
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

	tvCalendarStartupSyncEnabled := configService.GetString("TV_CALENDAR_STARTUP_SYNC_ENABLED")
	if tvCalendarStartupSyncEnabled == "" {
		tvCalendarStartupSyncEnabled = "true"
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

	systemService := services.NewSystemService()
	emailService := services.NewEmailService()
	telegramService := services.NewTelegramService()
	tvCalendarService := services.NewTVCalendarService()
	var rankingService *services.PlaybackRankingService
	if rankingCronEnabled == "true" {
		rankingService = services.NewPlaybackRankingService()
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

	if rankingCronEnabled == "true" {
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
		if tvCalendarStartupSyncEnabled == "true" {
			time.AfterFunc(15*time.Second, func() {
				count, err := tvCalendarService.SyncCalendar(context.Background(), services.DefaultTVCalendarWeekOffsets(), nil, false)
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
			count, err := tvCalendarService.SyncCalendar(context.Background(), services.DefaultTVCalendarWeekOffsets(), nil, false)
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

	c.Start()
	if rankingCronEnabled == "true" {
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
