package services

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"gorm.io/gorm"
)

type DeviceService struct {
	embyService *embyint.EmbyService
}

func NewDeviceService() *DeviceService {
	return &DeviceService{
		embyService: embyint.NewEmbyService(),
	}
}

type GetDevicesRequest struct {
	UserID        string `form:"userId"`
	ClientName    string `form:"clientName"`
	IsBlacklisted *bool  `form:"isBlacklisted"`
	Page          int    `form:"page" binding:"omitempty,min=1"`
	PageSize      int    `form:"pageSize" binding:"omitempty,min=1"`
}

type DeviceItem struct {
	DeviceID           string `json:"deviceId"`
	DeviceName         string `json:"deviceName"`
	ClientName         string `json:"clientName"`
	UserID             string `json:"userId,omitempty"`
	UserName           string `json:"userName,omitempty"`
	EmbyUserID         string `json:"embyUserId,omitempty"`
	IsActive           bool   `json:"isActive"`
	IsBlacklisted      bool   `json:"isBlacklisted"`
	BlacklistReason    string `json:"blacklistReason,omitempty"`
	LastActivityDate   string `json:"lastActivityDate,omitempty"`
	ApplicationVersion string `json:"applicationVersion,omitempty"`
	RemoteEndpoint     string `json:"remoteEndpoint,omitempty"`
}

type DeviceListResponse struct {
	Data       []DeviceItem `json:"data"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalPages int          `json:"totalPages"`
}

type ClientDistributionItem struct {
	ClientName string `json:"clientName"`
	Count      int    `json:"count"`
}

type TopDeviceItem struct {
	DeviceName string `json:"deviceName"`
	Count      int    `json:"count"`
}

type DeviceStats struct {
	ClientDistribution     []ClientDistributionItem `json:"clientDistribution"`
	TopDevices             []TopDeviceItem          `json:"topDevices"`
	BlacklistedClientCount int                      `json:"blacklistedClientCount"`
	ActiveSessionCount     int                      `json:"activeSessionCount"`
}

type DeviceActionsResponse struct {
	Data []models.DeviceAction `json:"data"`
}

func (s *DeviceService) GetDevices(req GetDevicesRequest) (*DeviceListResponse, error) {
	items, err := s.buildDeviceItems()
	if err != nil {
		return nil, err
	}

	clientFilter := normalizeClientName(req.ClientName)
	filtered := make([]DeviceItem, 0, len(items))
	for _, item := range items {
		if req.UserID != "" && item.UserID != req.UserID {
			continue
		}
		if clientFilter != "" && !strings.Contains(normalizeClientName(item.ClientName), clientFilter) {
			continue
		}
		if req.IsBlacklisted != nil && item.IsBlacklisted != *req.IsBlacklisted {
			continue
		}
		filtered = append(filtered, item)
	}

	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	total := int64(len(filtered))
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(pageSize)))
	}

	offset := (page - 1) * pageSize
	if offset > len(filtered) {
		offset = len(filtered)
	}
	end := offset + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}

	return &DeviceListResponse{
		Data:       filtered[offset:end],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (s *DeviceService) GetBlacklist() ([]models.ClientBlacklist, error) {
	var blacklists []models.ClientBlacklist
	if err := db.DB.Order("\"createdAt\" DESC").Find(&blacklists).Error; err != nil {
		return nil, errors.New("获取黑名单失败")
	}
	return blacklists, nil
}

func (s *DeviceService) AddClientToBlacklist(clientName, reason string) error {
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		return ErrDeviceClientNameRequired
	}
	reason = strings.TrimSpace(reason)
	normalized := normalizeClientName(clientName)

	var blacklist models.ClientBlacklist
	err := db.DB.Where("\"normalizedClientName\" = ?", normalized).First(&blacklist).Error
	if err == nil {
		blacklist.ClientName = clientName
		blacklist.Reason = reason
		blacklist.NormalizedClientName = normalized
		if err := db.DB.Save(&blacklist).Error; err != nil {
			return errors.New("更新黑名单失败")
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		blacklist = models.ClientBlacklist{
			ClientName:           clientName,
			NormalizedClientName: normalized,
			Reason:               reason,
		}
		if err := db.DB.Create(&blacklist).Error; err != nil {
			return errors.New("添加黑名单失败")
		}
	} else {
		return errors.New("添加黑名单失败")
	}

	s.recordDeviceAction("", "", clientName, "blacklist", reason)
	return nil
}

func (s *DeviceService) RemoveClientFromBlacklist(clientName string) error {
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		return ErrDeviceClientNameRequired
	}
	normalized := normalizeClientName(clientName)

	var blacklist models.ClientBlacklist
	if err := db.DB.Where("\"normalizedClientName\" = ?", normalized).First(&blacklist).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrClientBlacklistNotFound
		}
		return errors.New("移除黑名单失败")
	}

	if err := db.DB.Delete(&blacklist).Error; err != nil {
		return errors.New("移除黑名单失败")
	}

	s.recordDeviceAction("", "", blacklist.ClientName, "unblacklist", "")
	return nil
}

func (s *DeviceService) LogoutDevice(deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ErrDeviceIDRequired
	}

	var actionUserID string
	var actionClientName string
	items, err := s.buildDeviceItems()
	if err == nil {
		for _, item := range items {
			if item.DeviceID == deviceID {
				actionUserID = item.UserID
				actionClientName = item.ClientName
				break
			}
		}
	}

	if err := s.embyService.LogoutDevice(deviceID); err != nil {
		return err
	}

	s.recordDeviceAction(deviceID, actionUserID, actionClientName, "logout", "manual")
	return nil
}

func (s *DeviceService) LogoutBlacklistedDevices() (int, error) {
	items, err := s.buildDeviceItems()
	if err != nil {
		return 0, err
	}

	targets := make(map[string]DeviceItem)
	for _, item := range items {
		if item.IsBlacklisted && item.DeviceID != "" {
			targets[item.DeviceID] = item
		}
	}

	successCount := 0
	failedIDs := make([]string, 0)
	for deviceID, item := range targets {
		if err := s.embyService.LogoutDevice(deviceID); err != nil {
			failedIDs = append(failedIDs, deviceID)
			continue
		}
		successCount++
		s.recordDeviceAction(deviceID, item.UserID, item.ClientName, "logout", "blacklist")
	}

	if len(failedIDs) > 0 {
		return successCount, fmt.Errorf("部分设备注销失败：%s", strings.Join(failedIDs, ", "))
	}

	return successCount, nil
}

func (s *DeviceService) GetStats() (*DeviceStats, error) {
	items, err := s.buildDeviceItems()
	if err != nil {
		return nil, err
	}

	clientCounts := make(map[string]int)
	deviceCounts := make(map[string]int)
	for _, item := range items {
		clientName := strings.TrimSpace(item.ClientName)
		if clientName == "" {
			clientName = "Unknown"
		}
		clientCounts[clientName]++

		deviceName := strings.TrimSpace(item.DeviceName)
		if deviceName == "" {
			deviceName = item.DeviceID
		}
		if deviceName == "" {
			deviceName = "Unknown"
		}
		deviceCounts[deviceName]++
	}

	clientDistribution := make([]ClientDistributionItem, 0, len(clientCounts))
	for clientName, count := range clientCounts {
		clientDistribution = append(clientDistribution, ClientDistributionItem{
			ClientName: clientName,
			Count:      count,
		})
	}
	sort.Slice(clientDistribution, func(i, j int) bool {
		if clientDistribution[i].Count != clientDistribution[j].Count {
			return clientDistribution[i].Count > clientDistribution[j].Count
		}
		return clientDistribution[i].ClientName < clientDistribution[j].ClientName
	})

	topDevices := make([]TopDeviceItem, 0, len(deviceCounts))
	for deviceName, count := range deviceCounts {
		topDevices = append(topDevices, TopDeviceItem{
			DeviceName: deviceName,
			Count:      count,
		})
	}
	sort.Slice(topDevices, func(i, j int) bool {
		if topDevices[i].Count != topDevices[j].Count {
			return topDevices[i].Count > topDevices[j].Count
		}
		return topDevices[i].DeviceName < topDevices[j].DeviceName
	})
	if len(topDevices) > 10 {
		topDevices = topDevices[:10]
	}

	var blacklistedCount int64
	if err := db.DB.Model(&models.ClientBlacklist{}).Count(&blacklistedCount).Error; err != nil {
		return nil, errors.New("获取统计信息失败")
	}

	sessions, err := s.embyService.GetSessions()
	if err != nil {
		return nil, err
	}

	return &DeviceStats{
		ClientDistribution:     clientDistribution,
		TopDevices:             topDevices,
		BlacklistedClientCount: int(blacklistedCount),
		ActiveSessionCount:     len(sessions),
	}, nil
}

func (s *DeviceService) GetDeviceActions(limit int) (*DeviceActionsResponse, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	var actions []models.DeviceAction
	if err := db.DB.Order("\"createdAt\" DESC").Limit(limit).Find(&actions).Error; err != nil {
		return nil, errors.New("获取设备操作日志失败")
	}

	return &DeviceActionsResponse{
		Data: actions,
	}, nil
}

func (s *DeviceService) buildDeviceItems() ([]DeviceItem, error) {
	devices, devicesErr := s.embyService.GetDevices()
	sessions, sessionsErr := s.embyService.GetAllSessions()

	if devicesErr != nil && sessionsErr != nil {
		return nil, errors.New("获取设备列表失败")
	}

	var blacklists []models.ClientBlacklist
	if err := db.DB.Find(&blacklists).Error; err != nil {
		return nil, errors.New("获取设备列表失败")
	}
	blacklistMap := make(map[string]models.ClientBlacklist, len(blacklists))
	for _, blacklist := range blacklists {
		blacklistMap[blacklist.NormalizedClientName] = blacklist
	}

	entryMap := make(map[string]*DeviceItem)

	for _, device := range devices {
		deviceID := strings.TrimSpace(device.ID)
		if deviceID == "" {
			continue
		}

		entry := ensureDeviceEntry(entryMap, deviceID)
		if entry.DeviceName == "" && device.Name != "" {
			entry.DeviceName = device.Name
		}
		if entry.ClientName == "" && device.AppName != "" {
			entry.ClientName = device.AppName
		}
		if entry.UserName == "" && device.LastUserName != "" {
			entry.UserName = device.LastUserName
		}
		if entry.EmbyUserID == "" && device.LastUserID != "" {
			entry.EmbyUserID = device.LastUserID
		}
		if entry.LastActivityDate == "" {
			entry.LastActivityDate = firstNonEmpty(device.LastActivityDate, device.LastUsedDate, device.DateCreated)
		}
		if entry.ApplicationVersion == "" && device.AppVersion != "" {
			entry.ApplicationVersion = device.AppVersion
		}
	}

	for _, session := range sessions {
		deviceID := strings.TrimSpace(session.DeviceID)
		if deviceID == "" {
			continue
		}

		entry := ensureDeviceEntry(entryMap, deviceID)
		if session.DeviceName != "" {
			entry.DeviceName = session.DeviceName
		}
		if session.Client != "" {
			entry.ClientName = session.Client
		}
		if session.UserName != "" {
			entry.UserName = session.UserName
		}
		if session.UserID != "" {
			entry.EmbyUserID = session.UserID
		}
		if session.LastActivityDate != "" {
			entry.LastActivityDate = session.LastActivityDate
		}
		if session.ApplicationVersion != "" {
			entry.ApplicationVersion = session.ApplicationVersion
		}
		if session.RemoteEndPoint != "" {
			entry.RemoteEndpoint = session.RemoteEndPoint
		}
		if session.NowPlayingItem != nil {
			entry.IsActive = true
		}
	}

	embyUserIDs := make([]string, 0)
	embySeen := make(map[string]struct{})
	for _, entry := range entryMap {
		if entry.EmbyUserID == "" {
			continue
		}
		if _, ok := embySeen[entry.EmbyUserID]; ok {
			continue
		}
		embySeen[entry.EmbyUserID] = struct{}{}
		embyUserIDs = append(embyUserIDs, entry.EmbyUserID)
	}

	localUserByEmbyID := make(map[string]models.User)
	if len(embyUserIDs) > 0 {
		var users []models.User
		if err := db.DB.Where("\"embyId\" IN ?", embyUserIDs).Find(&users).Error; err != nil {
			return nil, errors.New("获取设备列表失败")
		}
		for _, user := range users {
			localUserByEmbyID[user.EmbyID] = user
		}
	}

	items := make([]DeviceItem, 0, len(entryMap))
	for _, entry := range entryMap {
		if entry.DeviceName == "" {
			entry.DeviceName = entry.DeviceID
		}
		if entry.ClientName == "" {
			entry.ClientName = "Unknown"
		}

		if entry.EmbyUserID != "" {
			if user, ok := localUserByEmbyID[entry.EmbyUserID]; ok {
				entry.UserID = user.ID
				if entry.UserName == "" {
					entry.UserName = user.Username
				}
			}
		}

		normalizedClientName := normalizeClientName(entry.ClientName)
		if blacklist, ok := blacklistMap[normalizedClientName]; ok {
			entry.IsBlacklisted = true
			entry.BlacklistReason = blacklist.Reason
		}

		items = append(items, *entry)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].IsActive != items[j].IsActive {
			return items[i].IsActive
		}
		iTime := parseDateTime(items[i].LastActivityDate)
		jTime := parseDateTime(items[j].LastActivityDate)
		if !iTime.Equal(jTime) {
			return iTime.After(jTime)
		}
		return items[i].DeviceName < items[j].DeviceName
	})

	return items, nil
}

func (s *DeviceService) recordDeviceAction(deviceID, userID, clientName, action, note string) {
	deviceAction := models.DeviceAction{
		DeviceID:   strings.TrimSpace(deviceID),
		UserID:     strings.TrimSpace(userID),
		ClientName: strings.TrimSpace(clientName),
		Action:     strings.TrimSpace(action),
		Note:       strings.TrimSpace(note),
	}
	if deviceAction.Action == "" {
		return
	}
	_ = db.DB.Create(&deviceAction).Error
}

func ensureDeviceEntry(entryMap map[string]*DeviceItem, deviceID string) *DeviceItem {
	if entry, ok := entryMap[deviceID]; ok {
		return entry
	}
	entry := &DeviceItem{
		DeviceID: deviceID,
	}
	entryMap[deviceID] = entry
	return entry
}

func normalizeClientName(clientName string) string {
	return strings.ToLower(strings.TrimSpace(clientName))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseDateTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.0000000Z",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
