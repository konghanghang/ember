package services

import devicepkg "github.com/konghang/ember/backend/internal/services/device"

type DeviceService = devicepkg.DeviceService
type GetDevicesRequest = devicepkg.GetDevicesRequest
type DeviceItem = devicepkg.DeviceItem
type DeviceListResponse = devicepkg.DeviceListResponse
type ClientDistributionItem = devicepkg.ClientDistributionItem
type TopDeviceItem = devicepkg.TopDeviceItem
type DeviceStats = devicepkg.DeviceStats
type DeviceActionsResponse = devicepkg.DeviceActionsResponse

var (
	ErrClientBlacklistNotFound  = devicepkg.ErrClientBlacklistNotFound
	ErrDeviceClientNameRequired = devicepkg.ErrDeviceClientNameRequired
	ErrDeviceIDRequired         = devicepkg.ErrDeviceIDRequired
)

func NewDeviceService() *DeviceService {
	return devicepkg.NewDeviceService()
}
