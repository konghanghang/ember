package device

import (
	"errors"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/konghang/ember/backend/internal/db"
	embyint "github.com/konghang/ember/backend/internal/integrations/emby"
	"github.com/konghang/ember/backend/internal/models"
	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"
)

type DeviceService struct {
	embyService           *embyint.EmbyService
	buildDeviceItemsFn    func() ([]DeviceItem, error)
	logoutDeviceFn        func(deviceID string) error
	findClientBlacklist   func(normalized string) (*models.ClientBlacklist, error)
	createClientBlacklist func(blacklist *models.ClientBlacklist) error
	updateClientBlacklist func(blacklist *models.ClientBlacklist) error
	deleteClientBlacklist func(blacklist *models.ClientBlacklist) error
	recordDeviceActionFn  func(action models.DeviceAction) error
}

func NewDeviceService() *DeviceService {
	service := &DeviceService{
		embyService: embyint.GetSharedService(),
	}
	service.applyDefaults()
	return service
}

// applyDefaults fills production dependencies while preserving test fakes injected on DeviceService.
func (s *DeviceService) applyDefaults() {
	if s.buildDeviceItemsFn == nil {
		s.buildDeviceItemsFn = s.buildDeviceItemsFromSources
	}
	if s.logoutDeviceFn == nil {
		s.logoutDeviceFn = s.logoutDevice
	}
	if s.findClientBlacklist == nil {
		s.findClientBlacklist = findClientBlacklist
	}
	if s.createClientBlacklist == nil {
		s.createClientBlacklist = createClientBlacklist
	}
	if s.updateClientBlacklist == nil {
		s.updateClientBlacklist = updateClientBlacklist
	}
	if s.deleteClientBlacklist == nil {
		s.deleteClientBlacklist = deleteClientBlacklist
	}
	if s.recordDeviceActionFn == nil {
		s.recordDeviceActionFn = recordDeviceAction
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
	if err := db.DB.Order("\"created_at\" DESC").Find(&blacklists).Error; err != nil {
		return nil, errors.New("获取黑名单失败")
	}
	return blacklists, nil
}

func (s *DeviceService) AddClientToBlacklist(clientName, reason, operatorID string) error {
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		return ErrDeviceClientNameRequired
	}
	reason = strings.TrimSpace(reason)
	normalized := normalizeClientName(clientName)

	s.applyDefaults()
	blacklist, err := s.findClientBlacklist(normalized)
	if err == nil {
		blacklist.ClientName = clientName
		blacklist.Reason = reason
		blacklist.NormalizedClientName = normalized
		if err := s.updateClientBlacklist(blacklist); err != nil {
			return errors.New("更新黑名单失败")
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		blacklist = &models.ClientBlacklist{
			ClientName:           clientName,
			NormalizedClientName: normalized,
			Reason:               reason,
		}
		if err := s.createClientBlacklist(blacklist); err != nil {
			return errors.New("添加黑名单失败")
		}
	} else {
		return errors.New("添加黑名单失败")
	}

	s.recordDeviceAction("", "", clientName, "blacklist", reason, operatorID)
	return nil
}

func (s *DeviceService) RemoveClientFromBlacklist(clientName, operatorID string) error {
	clientName = strings.TrimSpace(clientName)
	if clientName == "" {
		return ErrDeviceClientNameRequired
	}
	normalized := normalizeClientName(clientName)

	s.applyDefaults()
	blacklist, err := s.findClientBlacklist(normalized)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrClientBlacklistNotFound
		}
		return errors.New("移除黑名单失败")
	}

	if err := s.deleteClientBlacklist(blacklist); err != nil {
		return errors.New("移除黑名单失败")
	}

	s.recordDeviceAction("", "", blacklist.ClientName, "unblacklist", "", operatorID)
	return nil
}

func (s *DeviceService) LogoutDevice(deviceID, operatorID string) error {
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

	if err := s.logoutDeviceFn(deviceID); err != nil {
		return err
	}

	s.recordDeviceAction(deviceID, actionUserID, actionClientName, "logout", "manual", operatorID)
	return nil
}

// LogoutBlacklistedResult 批量注销黑名单设备的结构化结果
type LogoutBlacklistedResult struct {
	SuccessDeviceIDs []string             `json:"successDeviceIds"`
	FailedDeviceIDs  []LogoutFailedDevice `json:"failedDeviceIds"`
}

// LogoutFailedDevice 注销失败的设备
type LogoutFailedDevice struct {
	DeviceID string `json:"deviceId"`
	Error    string `json:"error"`
}

func (s *DeviceService) LogoutBlacklistedDevices(operatorID string) (*LogoutBlacklistedResult, error) {
	items, err := s.buildDeviceItems()
	if err != nil {
		return nil, err
	}

	targets := make(map[string]DeviceItem)
	for _, item := range items {
		if item.IsBlacklisted && item.DeviceID != "" {
			targets[item.DeviceID] = item
		}
	}

	result := &LogoutBlacklistedResult{
		SuccessDeviceIDs: make([]string, 0),
		FailedDeviceIDs:  make([]LogoutFailedDevice, 0),
	}
	for deviceID, item := range targets {
		if err := s.logoutDeviceFn(deviceID); err != nil {
			log.Printf("[Device] 黑名单设备注销失败 deviceId=%s err=%v", deviceID, err)
			result.FailedDeviceIDs = append(result.FailedDeviceIDs, LogoutFailedDevice{
				DeviceID: deviceID,
				Error:    err.Error(),
			})
			continue
		}
		result.SuccessDeviceIDs = append(result.SuccessDeviceIDs, deviceID)
		s.recordDeviceAction(deviceID, item.UserID, item.ClientName, "logout", "blacklist", operatorID)
	}

	return result, nil
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
	if err := db.DB.Order("\"created_at\" DESC").Limit(limit).Find(&actions).Error; err != nil {
		return nil, errors.New("获取设备操作日志失败")
	}

	return &DeviceActionsResponse{
		Data: actions,
	}, nil
}

// buildDeviceItems loads device items through the configured source, allowing tests to avoid Emby and DB calls.
func (s *DeviceService) buildDeviceItems() ([]DeviceItem, error) {
	s.applyDefaults()
	return s.buildDeviceItemsFn()
}

func (s *DeviceService) buildDeviceItemsFromSources() ([]DeviceItem, error) {
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
		if err := db.DB.Where("\"emby_id\" IN ?", embyUserIDs).Find(&users).Error; err != nil {
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

// logoutDevice calls the production Emby logout adapter.
func (s *DeviceService) logoutDevice(deviceID string) error {
	return s.embyService.LogoutDevice(deviceID)
}

func (s *DeviceService) recordDeviceAction(deviceID, userID, clientName, action, note, operatorID string) {
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
	if operatorID != "" {
		deviceAction.OperatorID = &operatorID
	}
	s.applyDefaults()
	if err := s.recordDeviceActionFn(deviceAction); err != nil {
		log.Printf("[Device] 记录操作日志失败 action=%s deviceId=%s err=%v", action, deviceID, err)
	}
}

// findClientBlacklist loads one blacklist row by normalized client name.
func findClientBlacklist(normalized string) (*models.ClientBlacklist, error) {
	var blacklist models.ClientBlacklist
	if err := db.DB.Where("\"normalized_client_name\" = ?", normalized).First(&blacklist).Error; err != nil {
		return nil, err
	}
	return &blacklist, nil
}

// createClientBlacklist persists a new client blacklist row.
func createClientBlacklist(blacklist *models.ClientBlacklist) error {
	return db.DB.Create(blacklist).Error
}

// updateClientBlacklist persists the mutable fields of an existing client blacklist row.
func updateClientBlacklist(blacklist *models.ClientBlacklist) error {
	return db.DB.Model(&models.ClientBlacklist{}).
		Where("id = ?", blacklist.ID).
		Updates(map[string]interface{}{
			"client_name":            blacklist.ClientName,
			"reason":                 blacklist.Reason,
			"normalized_client_name": blacklist.NormalizedClientName,
		}).Error
}

// deleteClientBlacklist removes a client blacklist row.
func deleteClientBlacklist(blacklist *models.ClientBlacklist) error {
	return db.DB.Delete(blacklist).Error
}

// recordDeviceAction persists a device operation audit entry.
func recordDeviceAction(action models.DeviceAction) error {
	return db.DB.Create(&action).Error
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

var clientNameVersionSuffix = regexp.MustCompile(`\s+v?\d+(\.\d+)*$`)

func normalizeClientName(clientName string) string {
	s := strings.ToLower(strings.TrimSpace(clientName))
	s = clientNameVersionSuffix.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\u3000", " ")
	s = norm.NFC.String(s)
	s = strings.Join(strings.Fields(s), " ")
	return s
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
