// This file was automatically generated. DO NOT EDIT.
// If you have any remark or suggestion do not hesitate to open an issue.

// Package baremetal provides methods and message types of the baremetal v1 API.
package baremetal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/scaleway/scaleway-sdk-go/internal/errors"
	"github.com/scaleway/scaleway-sdk-go/internal/marshaler"
	"github.com/scaleway/scaleway-sdk-go/internal/parameter"
	"github.com/scaleway/scaleway-sdk-go/namegenerator"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// always import dependencies
var (
	_ fmt.Stringer
	_ json.Unmarshaler
	_ url.URL
	_ net.IP
	_ http.Header
	_ bytes.Reader
	_ time.Time
	_ = strings.Join

	_ scw.ScalewayRequest
	_ marshaler.Duration
	_ scw.File
	_ = parameter.AddToQuery
	_ = namegenerator.GetRandomName
)

// API: this API allows to manage your Bare metal server
type API struct {
	client *scw.Client
}

// NewAPI returns a API object from a Scaleway client.
func NewAPI(client *scw.Client) *API {
	return &API{
		client: client,
	}
}

type IPReverseStatus string

const (
	// IPReverseStatusUnknown is [insert doc].
	IPReverseStatusUnknown = IPReverseStatus("unknown")
	// IPReverseStatusPending is [insert doc].
	IPReverseStatusPending = IPReverseStatus("pending")
	// IPReverseStatusActive is [insert doc].
	IPReverseStatusActive = IPReverseStatus("active")
	// IPReverseStatusError is [insert doc].
	IPReverseStatusError = IPReverseStatus("error")
)

func (enum IPReverseStatus) String() string {
	if enum == "" {
		// return default value if empty
		return "unknown"
	}
	return string(enum)
}

func (enum IPReverseStatus) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *IPReverseStatus) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = IPReverseStatus(IPReverseStatus(tmp).String())
	return nil
}

type IPVersion string

const (
	// IPVersionIPv4 is [insert doc].
	IPVersionIPv4 = IPVersion("IPv4")
	// IPVersionIPv6 is [insert doc].
	IPVersionIPv6 = IPVersion("IPv6")
)

func (enum IPVersion) String() string {
	if enum == "" {
		// return default value if empty
		return "IPv4"
	}
	return string(enum)
}

func (enum IPVersion) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *IPVersion) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = IPVersion(IPVersion(tmp).String())
	return nil
}

type ListServerEventsRequestOrderBy string

const (
	// ListServerEventsRequestOrderByCreatedAtAsc is [insert doc].
	ListServerEventsRequestOrderByCreatedAtAsc = ListServerEventsRequestOrderBy("created_at_asc")
	// ListServerEventsRequestOrderByCreatedAtDesc is [insert doc].
	ListServerEventsRequestOrderByCreatedAtDesc = ListServerEventsRequestOrderBy("created_at_desc")
)

func (enum ListServerEventsRequestOrderBy) String() string {
	if enum == "" {
		// return default value if empty
		return "created_at_asc"
	}
	return string(enum)
}

func (enum ListServerEventsRequestOrderBy) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ListServerEventsRequestOrderBy) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ListServerEventsRequestOrderBy(ListServerEventsRequestOrderBy(tmp).String())
	return nil
}

type ListServersRequestOrderBy string

const (
	// ListServersRequestOrderByCreatedAtAsc is [insert doc].
	ListServersRequestOrderByCreatedAtAsc = ListServersRequestOrderBy("created_at_asc")
	// ListServersRequestOrderByCreatedAtDesc is [insert doc].
	ListServersRequestOrderByCreatedAtDesc = ListServersRequestOrderBy("created_at_desc")
)

func (enum ListServersRequestOrderBy) String() string {
	if enum == "" {
		// return default value if empty
		return "created_at_asc"
	}
	return string(enum)
}

func (enum ListServersRequestOrderBy) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ListServersRequestOrderBy) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ListServersRequestOrderBy(ListServersRequestOrderBy(tmp).String())
	return nil
}

type ListSettingsRequestOrderBy string

const (
	// ListSettingsRequestOrderByCreatedAtAsc is [insert doc].
	ListSettingsRequestOrderByCreatedAtAsc = ListSettingsRequestOrderBy("created_at_asc")
	// ListSettingsRequestOrderByCreatedAtDesc is [insert doc].
	ListSettingsRequestOrderByCreatedAtDesc = ListSettingsRequestOrderBy("created_at_desc")
)

func (enum ListSettingsRequestOrderBy) String() string {
	if enum == "" {
		// return default value if empty
		return "created_at_asc"
	}
	return string(enum)
}

func (enum ListSettingsRequestOrderBy) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ListSettingsRequestOrderBy) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ListSettingsRequestOrderBy(ListSettingsRequestOrderBy(tmp).String())
	return nil
}

type OfferStock string

const (
	// OfferStockEmpty is [insert doc].
	OfferStockEmpty = OfferStock("empty")
	// OfferStockLow is [insert doc].
	OfferStockLow = OfferStock("low")
	// OfferStockAvailable is [insert doc].
	OfferStockAvailable = OfferStock("available")
)

func (enum OfferStock) String() string {
	if enum == "" {
		// return default value if empty
		return "empty"
	}
	return string(enum)
}

func (enum OfferStock) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *OfferStock) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = OfferStock(OfferStock(tmp).String())
	return nil
}

type OfferSubscriptionPeriod string

const (
	// OfferSubscriptionPeriodUnknownSubscriptionPeriod is [insert doc].
	OfferSubscriptionPeriodUnknownSubscriptionPeriod = OfferSubscriptionPeriod("unknown_subscription_period")
	// OfferSubscriptionPeriodHourly is [insert doc].
	OfferSubscriptionPeriodHourly = OfferSubscriptionPeriod("hourly")
	// OfferSubscriptionPeriodMonthly is [insert doc].
	OfferSubscriptionPeriodMonthly = OfferSubscriptionPeriod("monthly")
)

func (enum OfferSubscriptionPeriod) String() string {
	if enum == "" {
		// return default value if empty
		return "unknown_subscription_period"
	}
	return string(enum)
}

func (enum OfferSubscriptionPeriod) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *OfferSubscriptionPeriod) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = OfferSubscriptionPeriod(OfferSubscriptionPeriod(tmp).String())
	return nil
}

type ServerBootType string

const (
	// ServerBootTypeUnknownBootType is [insert doc].
	ServerBootTypeUnknownBootType = ServerBootType("unknown_boot_type")
	// ServerBootTypeNormal is [insert doc].
	ServerBootTypeNormal = ServerBootType("normal")
	// ServerBootTypeRescue is [insert doc].
	ServerBootTypeRescue = ServerBootType("rescue")
)

func (enum ServerBootType) String() string {
	if enum == "" {
		// return default value if empty
		return "unknown_boot_type"
	}
	return string(enum)
}

func (enum ServerBootType) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ServerBootType) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ServerBootType(ServerBootType(tmp).String())
	return nil
}

type ServerInstallStatus string

const (
	// ServerInstallStatusUnknown is [insert doc].
	ServerInstallStatusUnknown = ServerInstallStatus("unknown")
	// ServerInstallStatusToInstall is [insert doc].
	ServerInstallStatusToInstall = ServerInstallStatus("to_install")
	// ServerInstallStatusInstalling is [insert doc].
	ServerInstallStatusInstalling = ServerInstallStatus("installing")
	// ServerInstallStatusCompleted is [insert doc].
	ServerInstallStatusCompleted = ServerInstallStatus("completed")
	// ServerInstallStatusError is [insert doc].
	ServerInstallStatusError = ServerInstallStatus("error")
)

func (enum ServerInstallStatus) String() string {
	if enum == "" {
		// return default value if empty
		return "unknown"
	}
	return string(enum)
}

func (enum ServerInstallStatus) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ServerInstallStatus) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ServerInstallStatus(ServerInstallStatus(tmp).String())
	return nil
}

type ServerOptionOptionStatus string

const (
	// ServerOptionOptionStatusOptionStatusUnknown is [insert doc].
	ServerOptionOptionStatusOptionStatusUnknown = ServerOptionOptionStatus("option_status_unknown")
	// ServerOptionOptionStatusOptionStatusEnable is [insert doc].
	ServerOptionOptionStatusOptionStatusEnable = ServerOptionOptionStatus("option_status_enable")
	// ServerOptionOptionStatusOptionStatusEnabling is [insert doc].
	ServerOptionOptionStatusOptionStatusEnabling = ServerOptionOptionStatus("option_status_enabling")
	// ServerOptionOptionStatusOptionStatusDisabling is [insert doc].
	ServerOptionOptionStatusOptionStatusDisabling = ServerOptionOptionStatus("option_status_disabling")
	// ServerOptionOptionStatusOptionStatusError is [insert doc].
	ServerOptionOptionStatusOptionStatusError = ServerOptionOptionStatus("option_status_error")
)

func (enum ServerOptionOptionStatus) String() string {
	if enum == "" {
		// return default value if empty
		return "option_status_unknown"
	}
	return string(enum)
}

func (enum ServerOptionOptionStatus) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ServerOptionOptionStatus) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ServerOptionOptionStatus(ServerOptionOptionStatus(tmp).String())
	return nil
}

type ServerPingStatus string

const (
	// ServerPingStatusPingStatusUnknown is [insert doc].
	ServerPingStatusPingStatusUnknown = ServerPingStatus("ping_status_unknown")
	// ServerPingStatusPingStatusUp is [insert doc].
	ServerPingStatusPingStatusUp = ServerPingStatus("ping_status_up")
	// ServerPingStatusPingStatusDown is [insert doc].
	ServerPingStatusPingStatusDown = ServerPingStatus("ping_status_down")
)

func (enum ServerPingStatus) String() string {
	if enum == "" {
		// return default value if empty
		return "ping_status_unknown"
	}
	return string(enum)
}

func (enum ServerPingStatus) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ServerPingStatus) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ServerPingStatus(ServerPingStatus(tmp).String())
	return nil
}

type ServerStatus string

const (
	// ServerStatusUnknown is [insert doc].
	ServerStatusUnknown = ServerStatus("unknown")
	// ServerStatusDelivering is [insert doc].
	ServerStatusDelivering = ServerStatus("delivering")
	// ServerStatusReady is [insert doc].
	ServerStatusReady = ServerStatus("ready")
	// ServerStatusStopping is [insert doc].
	ServerStatusStopping = ServerStatus("stopping")
	// ServerStatusStopped is [insert doc].
	ServerStatusStopped = ServerStatus("stopped")
	// ServerStatusStarting is [insert doc].
	ServerStatusStarting = ServerStatus("starting")
	// ServerStatusError is [insert doc].
	ServerStatusError = ServerStatus("error")
	// ServerStatusDeleting is [insert doc].
	ServerStatusDeleting = ServerStatus("deleting")
	// ServerStatusLocked is [insert doc].
	ServerStatusLocked = ServerStatus("locked")
	// ServerStatusOutOfStock is [insert doc].
	ServerStatusOutOfStock = ServerStatus("out_of_stock")
	// ServerStatusOrdered is [insert doc].
	ServerStatusOrdered = ServerStatus("ordered")
)

func (enum ServerStatus) String() string {
	if enum == "" {
		// return default value if empty
		return "unknown"
	}
	return string(enum)
}

func (enum ServerStatus) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *ServerStatus) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = ServerStatus(ServerStatus(tmp).String())
	return nil
}

type SettingType string

const (
	// SettingTypeUnknown is [insert doc].
	SettingTypeUnknown = SettingType("unknown")
	// SettingTypeSMTP is [insert doc].
	SettingTypeSMTP = SettingType("smtp")
)

func (enum SettingType) String() string {
	if enum == "" {
		// return default value if empty
		return "unknown"
	}
	return string(enum)
}

func (enum SettingType) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, enum)), nil
}

func (enum *SettingType) UnmarshalJSON(data []byte) error {
	tmp := ""

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*enum = SettingType(SettingType(tmp).String())
	return nil
}

// BMCAccess: bmc access
type BMCAccess struct {
	// URL: URL to access to the server console
	URL string `json:"url"`
	// Login: the login to use for the BMC (Baseboard Management Controller) access authentification
	Login string `json:"login"`
	// Password: the password to use for the BMC (Baseboard Management Controller) access authentification
	Password string `json:"password"`
	// ExpiresAt: the date after which the BMC (Baseboard Management Controller) access will be closed
	ExpiresAt *time.Time `json:"expires_at"`
}

// CPU: cpu
type CPU struct {
	// Name: name of the CPU
	Name string `json:"name"`
	// CoreCount: number of cores of the CPU
	CoreCount uint32 `json:"core_count"`
	// ThreadCount: number of threads of the CPU
	ThreadCount uint32 `json:"thread_count"`

	Frequency uint32 `json:"frequency"`
}

type CreateServerRequestInstall struct {
	OsID string `json:"os_id"`

	Hostname string `json:"hostname"`

	SSHKeyIDs []string `json:"ssh_key_ids"`

	User *string `json:"user"`

	Password *string `json:"password"`

	ServiceUser *string `json:"service_user"`

	ServicePassword *string `json:"service_password"`
}

// Disk: disk
type Disk struct {
	// Capacity: capacity of the disk in bytes
	Capacity scw.Size `json:"capacity"`
	// Type: type of the disk
	Type string `json:"type"`
}

// GetServerMetricsResponse: get server metrics response
type GetServerMetricsResponse struct {
	// Pings: timeseries of ping on the server
	Pings *scw.TimeSeries `json:"pings"`
}

// IP: ip
type IP struct {
	// ID: ID of the IP
	ID string `json:"id"`
	// Address: address of the IP
	Address net.IP `json:"address"`
	// Reverse: reverse IP value
	Reverse string `json:"reverse"`
	// Version: version of IP (v4 or v6)
	//
	// Default value: IPv4
	Version IPVersion `json:"version"`
	// ReverseStatus: status of the reverse
	//
	// Default value: unknown
	ReverseStatus IPReverseStatus `json:"reverse_status"`
	// ReverseStatusMessage: a message related to the reverse status, in case of an error for example
	ReverseStatusMessage string `json:"reverse_status_message"`
}

// ListOSResponse: list os response
type ListOSResponse struct {
	// TotalCount: total count of matching OS
	TotalCount uint32 `json:"total_count"`
	// Os: oS that match filters
	Os []*OS `json:"os"`
}

// ListOffersResponse: list offers response
type ListOffersResponse struct {
	// TotalCount: total count of matching offers
	TotalCount uint32 `json:"total_count"`
	// Offers: offers that match filters
	Offers []*Offer `json:"offers"`
}

// ListOptionsResponse: list options response
type ListOptionsResponse struct {
	// TotalCount: total count of matching options
	TotalCount uint32 `json:"total_count"`
	// Options: options that match filters
	Options []*Option `json:"options"`
}

// ListServerEventsResponse: list server events response
type ListServerEventsResponse struct {
	// TotalCount: total count of matching events
	TotalCount uint32 `json:"total_count"`
	// Events: server events that match filters
	Events []*ServerEvent `json:"events"`
}

// ListServersResponse: list servers response
type ListServersResponse struct {
	// TotalCount: total count of matching servers
	TotalCount uint32 `json:"total_count"`
	// Servers: servers that match filters
	Servers []*Server `json:"servers"`
}

// ListSettingsResponse: list settings response
type ListSettingsResponse struct {
	// TotalCount: total count of matching sttings
	TotalCount uint32 `json:"total_count"`
	// Settings: settings that match filters
	Settings []*Setting `json:"settings"`
}

// Memory: memory
type Memory struct {
	Capacity scw.Size `json:"capacity"`

	Type string `json:"type"`

	Frequency uint32 `json:"frequency"`

	IsEcc bool `json:"is_ecc"`
}

// OS: os
type OS struct {
	// ID: ID of the OS
	ID string `json:"id"`
	// Name: name of the OS
	Name string `json:"name"`
	// Version: version of the OS
	Version string `json:"version"`
	// LogoURL: URL of this os's logo
	LogoURL string `json:"logo_url"`
	// SSH: define the SSH requirements to install the OS
	SSH *OSOSField `json:"ssh"`
	// User: define the username requirements to install the OS
	User *OSOSField `json:"user"`
	// Password: define the password requirements to install the OS
	Password *OSOSField `json:"password"`
	// ServiceUser: define the username requirements to install the service
	ServiceUser *OSOSField `json:"service_user"`
	// ServicePassword: define the password requirements to install the service
	ServicePassword *OSOSField `json:"service_password"`
}

type OSOSField struct {
	Editable bool `json:"editable"`

	Required bool `json:"required"`

	DefaultValue *string `json:"default_value"`
}

// Offer: offer
type Offer struct {
	// ID: ID of the offer
	ID string `json:"id"`
	// Name: name of the offer
	Name string `json:"name"`
	// Stock: stock level
	//
	// Default value: empty
	Stock OfferStock `json:"stock"`
	// Bandwidth: bandwidth available in bits/s with the offer
	Bandwidth uint64 `json:"bandwidth"`
	// CommercialRange: commercial range of the offer
	CommercialRange string `json:"commercial_range"`
	// PricePerHour: price of the offer for the next 60 minutes (a server order at 11h32 will be payed until 12h32)
	PricePerHour *scw.Money `json:"price_per_hour"`
	// PricePerMonth: price of the offer per months
	PricePerMonth *scw.Money `json:"price_per_month"`
	// Disks: disks specifications of the offer
	Disks []*Disk `json:"disks"`
	// Enable: true if the offer is currently available
	Enable bool `json:"enable"`
	// CPUs: CPU specifications of the offer
	CPUs []*CPU `json:"cpus"`
	// Memories: memory specifications of the offer
	Memories []*Memory `json:"memories"`
	// QuotaName: name of the quota associated to the offer
	QuotaName string `json:"quota_name"`
	// PersistentMemories: persistent memory specifications of the offer
	PersistentMemories []*PersistentMemory `json:"persistent_memories"`
	// RaidControllers: raid controller specifications of the offer
	RaidControllers []*RaidController `json:"raid_controllers"`
	// IncompatibleOsIDs: array of incompatible OS ids
	IncompatibleOsIDs []string `json:"incompatible_os_ids"`
	// SubscriptionPeriod: period of subscription for the offer
	//
	// Default value: unknown_subscription_period
	SubscriptionPeriod OfferSubscriptionPeriod `json:"subscription_period"`
	// OperationPath: operation path of the service
	OperationPath string `json:"operation_path"`
	// Fee: fee to pay on order
	Fee *scw.Money `json:"fee"`
	// Options: options available on offer
	Options []*OfferOptionOffer `json:"options"`
}

type OfferOptionOffer struct {
	ID string `json:"id"`

	Name string `json:"name"`

	Price *scw.Money `json:"price"`

	Enabled bool `json:"enabled"`
	// SubscriptionPeriod:
	//
	// Default value: unknown_subscription_period
	SubscriptionPeriod OfferSubscriptionPeriod `json:"subscription_period"`

	Manageable bool `json:"manageable"`

	OsID *string `json:"os_id"`
}

// Option: option
type Option struct {
	// ID: ID of the option
	ID string `json:"id"`
	// Name: name of the option
	Name string `json:"name"`
	// Manageable: is false if the option could not be remove
	Manageable bool `json:"manageable"`
}

type PersistentMemory struct {
	Capacity scw.Size `json:"capacity"`

	Type string `json:"type"`

	Frequency uint32 `json:"frequency"`
}

type RaidController struct {
	Model string `json:"model"`

	RaidLevel []string `json:"raid_level"`
}

// Server: server
type Server struct {
	// ID: ID of the server
	ID string `json:"id"`
	// OrganizationID: organization ID the server is attached to
	OrganizationID string `json:"organization_id"`
	// ProjectID: project ID the server is attached to
	ProjectID string `json:"project_id"`
	// Name: name of the server
	Name string `json:"name"`
	// Description: description of the server
	Description string `json:"description"`
	// UpdatedAt: date of last modification of the server
	UpdatedAt *time.Time `json:"updated_at"`
	// CreatedAt: date of creation of the server
	CreatedAt *time.Time `json:"created_at"`
	// Status: status of the server
	//
	// Default value: unknown
	Status ServerStatus `json:"status"`
	// OfferID: offer ID of the server
	OfferID string `json:"offer_id"`
	// OfferName: offer name of the server
	OfferName string `json:"offer_name"`
	// Tags: array of customs tags attached to the server
	Tags []string `json:"tags"`
	// IPs: array of IPs attached to the server
	IPs []*IP `json:"ips"`
	// Domain: domain of the server
	Domain string `json:"domain"`
	// BootType: boot type of the server
	//
	// Default value: unknown_boot_type
	BootType ServerBootType `json:"boot_type"`
	// Zone: the zone in which is the server
	Zone scw.Zone `json:"zone"`
	// Install: configuration of installation
	Install *ServerInstall `json:"install"`
	// PingStatus: server status of ping
	//
	// Default value: ping_status_unknown
	PingStatus ServerPingStatus `json:"ping_status"`
	// Options: options enabled on server
	Options []*ServerOption `json:"options"`
}

// ServerEvent: server event
type ServerEvent struct {
	// ID: ID of the server for whom the action will be applied
	ID string `json:"id"`
	// Action: the action that will be applied to the server
	Action string `json:"action"`
	// UpdatedAt: date of last modification of the action
	UpdatedAt *time.Time `json:"updated_at"`
	// CreatedAt: date of creation of the action
	CreatedAt *time.Time `json:"created_at"`
}

type ServerInstall struct {
	OsID string `json:"os_id"`

	Hostname string `json:"hostname"`

	SSHKeyIDs []string `json:"ssh_key_ids"`
	// Status:
	//
	// Default value: unknown
	Status ServerInstallStatus `json:"status"`

	User string `json:"user"`

	ServiceUser string `json:"service_user"`

	ServiceURL string `json:"service_url"`
}

type ServerOption struct {
	ID string `json:"id"`

	Name string `json:"name"`
	// Status:
	//
	// Default value: option_status_unknown
	Status ServerOptionOptionStatus `json:"status"`

	Manageable bool `json:"manageable"`
}

// Setting: setting
type Setting struct {
	// ID: ID of the setting
	ID string `json:"id"`
	// Type: type of the setting
	//
	// Default value: unknown
	Type SettingType `json:"type"`
	// ProjectID: ID of the project ID
	ProjectID string `json:"project_id"`
	// Enabled: the setting is enable or disable
	Enabled bool `json:"enabled"`
}

// Service API

type ListServersRequest struct {
	Zone scw.Zone `json:"-"`
	// Page: page number
	Page *int32 `json:"-"`
	// PageSize: number of server per page
	PageSize *uint32 `json:"-"`
	// OrderBy: order of the servers
	//
	// Default value: created_at_asc
	OrderBy ListServersRequestOrderBy `json:"-"`
	// Tags: filter by tags
	Tags []string `json:"-"`
	// Status: filter by status
	Status []string `json:"-"`
	// Name: filter by name
	Name *string `json:"-"`
	// OrganizationID: filter by organization ID
	OrganizationID *string `json:"-"`
	// ProjectID: filter by project ID
	ProjectID *string `json:"-"`
	// OptionID: filter by option ID
	OptionID *string `json:"-"`
}

// ListServers: list elastic metal servers for organization
//
// List elastic metal servers for organization.
func (s *API) ListServers(req *ListServersRequest, opts ...scw.RequestOption) (*ListServersResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	defaultPageSize, exist := s.client.GetDefaultPageSize()
	if (req.PageSize == nil || *req.PageSize == 0) && exist {
		req.PageSize = &defaultPageSize
	}

	query := url.Values{}
	parameter.AddToQuery(query, "page", req.Page)
	parameter.AddToQuery(query, "page_size", req.PageSize)
	parameter.AddToQuery(query, "order_by", req.OrderBy)
	parameter.AddToQuery(query, "tags", req.Tags)
	parameter.AddToQuery(query, "status", req.Status)
	parameter.AddToQuery(query, "name", req.Name)
	parameter.AddToQuery(query, "organization_id", req.OrganizationID)
	parameter.AddToQuery(query, "project_id", req.ProjectID)
	parameter.AddToQuery(query, "option_id", req.OptionID)

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers",
		Query:   query,
		Headers: http.Header{},
	}

	var resp ListServersResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
}

// GetServer: get a specific elastic metal server
//
// Get the server associated with the given ID.
func (s *API) GetServer(req *GetServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "",
		Headers: http.Header{},
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type CreateServerRequest struct {
	Zone scw.Zone `json:"-"`
	// OfferID: offer ID of the new server
	OfferID string `json:"offer_id"`
	// Deprecated: OrganizationID: organization ID with which the server will be created
	// Precisely one of OrganizationID, ProjectID must be set.
	OrganizationID *string `json:"organization_id,omitempty"`
	// ProjectID: project ID with which the server will be created
	// Precisely one of OrganizationID, ProjectID must be set.
	ProjectID *string `json:"project_id,omitempty"`
	// Name: name of the server (≠hostname)
	Name string `json:"name"`
	// Description: description associated to the server, max 255 characters
	Description string `json:"description"`
	// Tags: tags to associate to the server
	Tags []string `json:"tags"`

	Install *CreateServerRequestInstall `json:"install"`
	// OptionIDs: iDs of options to enable on server
	OptionIDs []string `json:"option_ids"`
}

// CreateServer: create a elastic metal server
//
// Create a new elastic metal server. Once the server is created, you probably want to install an OS.
func (s *API) CreateServer(req *CreateServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	defaultProjectID, exist := s.client.GetDefaultProjectID()
	if exist && req.OrganizationID == nil && req.ProjectID == nil {
		req.ProjectID = &defaultProjectID
	}

	defaultOrganizationID, exist := s.client.GetDefaultOrganizationID()
	if exist && req.OrganizationID == nil && req.ProjectID == nil {
		req.OrganizationID = &defaultOrganizationID
	}

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type UpdateServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server to update
	ServerID string `json:"-"`
	// Name: name of the server (≠hostname), not updated if null
	Name *string `json:"name"`
	// Description: description associated to the server, max 255 characters, not updated if null
	Description *string `json:"description"`
	// Tags: tags associated to the server, not updated if null
	Tags *[]string `json:"tags"`
}

// UpdateServer: update a elastic metal server
//
// Update the server associated with the given ID.
func (s *API) UpdateServer(req *UpdateServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "PATCH",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type InstallServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: server ID to install
	ServerID string `json:"-"`
	// OsID: ID of the OS to install on the server
	OsID string `json:"os_id"`
	// Hostname: hostname of the server
	Hostname string `json:"hostname"`
	// SSHKeyIDs: SSH key IDs authorized on the server
	SSHKeyIDs []string `json:"ssh_key_ids"`
	// User: user used for the installation
	User *string `json:"user"`
	// Password: password used for the installation
	Password *string `json:"password"`
	// ServiceUser: user used for the service to install
	ServiceUser *string `json:"service_user"`
	// ServicePassword: password used for the service to install
	ServicePassword *string `json:"service_password"`
}

// InstallServer: install a elastic metal server
//
// Install an OS on the server associated with the given ID.
func (s *API) InstallServer(req *InstallServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/install",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetServerMetricsRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: server ID to get the metrics
	ServerID string `json:"-"`
}

// GetServerMetrics: return server metrics
//
// Give the ping status on the server associated with the given ID.
func (s *API) GetServerMetrics(req *GetServerMetricsRequest, opts ...scw.RequestOption) (*GetServerMetricsResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/metrics",
		Headers: http.Header{},
	}

	var resp GetServerMetricsResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type DeleteServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server to delete
	ServerID string `json:"-"`
}

// DeleteServer: delete a elastic metal server
//
// Delete the server associated with the given ID.
func (s *API) DeleteServer(req *DeleteServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "DELETE",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "",
		Headers: http.Header{},
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type RebootServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server to reboot
	ServerID string `json:"-"`
	// BootType: the type of boot
	//
	// Default value: unknown_boot_type
	BootType ServerBootType `json:"boot_type"`
}

// RebootServer: reboot a elastic metal server
//
// Reboot the server associated with the given ID, use boot param to reboot in rescue.
func (s *API) RebootServer(req *RebootServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/reboot",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type StartServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server to start
	ServerID string `json:"-"`
	// BootType: the type of boot
	//
	// Default value: unknown_boot_type
	BootType ServerBootType `json:"boot_type"`
}

// StartServer: start a elastic metal server
//
// Start the server associated with the given ID.
func (s *API) StartServer(req *StartServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/start",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type StopServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server to stop
	ServerID string `json:"-"`
}

// StopServer: stop a elastic metal server
//
// Stop the server associated with the given ID.
func (s *API) StopServer(req *StopServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/stop",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type ListServerEventsRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server events searched
	ServerID string `json:"-"`
	// Page: page number
	Page *int32 `json:"-"`
	// PageSize: number of server events per page
	PageSize *uint32 `json:"-"`
	// OrderBy: order of the server events
	//
	// Default value: created_at_asc
	OrderBy ListServerEventsRequestOrderBy `json:"-"`
}

// ListServerEvents: list server events
//
// List events associated to the given server ID.
func (s *API) ListServerEvents(req *ListServerEventsRequest, opts ...scw.RequestOption) (*ListServerEventsResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	defaultPageSize, exist := s.client.GetDefaultPageSize()
	if (req.PageSize == nil || *req.PageSize == 0) && exist {
		req.PageSize = &defaultPageSize
	}

	query := url.Values{}
	parameter.AddToQuery(query, "page", req.Page)
	parameter.AddToQuery(query, "page_size", req.PageSize)
	parameter.AddToQuery(query, "order_by", req.OrderBy)

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/events",
		Query:   query,
		Headers: http.Header{},
	}

	var resp ListServerEventsResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type StartBMCAccessRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
	// IP: the IP authorized to connect to the given server
	IP net.IP `json:"ip"`
}

// StartBMCAccess: unvailable - Start BMC (Baseboard Management Controller) access for a given elastic metal server
//
// Unvailable - Start BMC (Baseboard Management Controller) access associated with the given ID.
// The BMC (Baseboard Management Controller) access is available one hour after the installation of the server.
//
func (s *API) StartBMCAccess(req *StartBMCAccessRequest, opts ...scw.RequestOption) (*BMCAccess, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/bmc-access",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp BMCAccess

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetBMCAccessRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
}

// GetBMCAccess: get BMC (Baseboard Management Controller) access for a given elastic metal server
//
// Get the BMC (Baseboard Management Controller) access associated with the given ID.
func (s *API) GetBMCAccess(req *GetBMCAccessRequest, opts ...scw.RequestOption) (*BMCAccess, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/bmc-access",
		Headers: http.Header{},
	}

	var resp BMCAccess

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type StopBMCAccessRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
}

// StopBMCAccess: stop BMC (Baseboard Management Controller) access for a given elastic metal server
//
// Stop BMC (Baseboard Management Controller) access associated with the given ID.
func (s *API) StopBMCAccess(req *StopBMCAccessRequest, opts ...scw.RequestOption) error {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return errors.New("field ServerID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "DELETE",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/bmc-access",
		Headers: http.Header{},
	}

	err = s.client.Do(scwReq, nil, opts...)
	if err != nil {
		return err
	}
	return nil
}

type UpdateIPRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
	// IPID: ID of the IP to update
	IPID string `json:"-"`
	// Reverse: new reverse IP to update, not updated if null
	Reverse *string `json:"reverse"`
}

// UpdateIP: update IP
//
// Configure ip associated with the given server ID and ipID. You can use this method to set a reverse dns for an IP.
func (s *API) UpdateIP(req *UpdateIPRequest, opts ...scw.RequestOption) (*IP, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	if fmt.Sprint(req.IPID) == "" {
		return nil, errors.New("field IPID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "PATCH",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/ips/" + fmt.Sprint(req.IPID) + "",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp IP

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type AddOptionServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
	// OptionID: ID of the option to add
	OptionID string `json:"-"`
}

// AddOptionServer: add server option
//
// Add an option to a specific server.
func (s *API) AddOptionServer(req *AddOptionServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	if fmt.Sprint(req.OptionID) == "" {
		return nil, errors.New("field OptionID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "POST",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/options/" + fmt.Sprint(req.OptionID) + "",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type DeleteOptionServerRequest struct {
	Zone scw.Zone `json:"-"`
	// ServerID: ID of the server
	ServerID string `json:"-"`
	// OptionID: ID of the option to delete
	OptionID string `json:"-"`
}

// DeleteOptionServer: delete server option
//
// Delete an option from a specific server.
func (s *API) DeleteOptionServer(req *DeleteOptionServerRequest, opts ...scw.RequestOption) (*Server, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.ServerID) == "" {
		return nil, errors.New("field ServerID cannot be empty in request")
	}

	if fmt.Sprint(req.OptionID) == "" {
		return nil, errors.New("field OptionID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "DELETE",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/servers/" + fmt.Sprint(req.ServerID) + "/options/" + fmt.Sprint(req.OptionID) + "",
		Headers: http.Header{},
	}

	var resp Server

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type ListOffersRequest struct {
	Zone scw.Zone `json:"-"`
	// Page: page number
	Page *int32 `json:"-"`
	// PageSize: number of offers per page
	PageSize *uint32 `json:"-"`
	// SubscriptionPeriod: period of subscription to filter offers
	//
	// Default value: unknown_subscription_period
	SubscriptionPeriod OfferSubscriptionPeriod `json:"-"`
}

// ListOffers: list offers
//
// List all available server offers.
func (s *API) ListOffers(req *ListOffersRequest, opts ...scw.RequestOption) (*ListOffersResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	defaultPageSize, exist := s.client.GetDefaultPageSize()
	if (req.PageSize == nil || *req.PageSize == 0) && exist {
		req.PageSize = &defaultPageSize
	}

	query := url.Values{}
	parameter.AddToQuery(query, "page", req.Page)
	parameter.AddToQuery(query, "page_size", req.PageSize)
	parameter.AddToQuery(query, "subscription_period", req.SubscriptionPeriod)

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/offers",
		Query:   query,
		Headers: http.Header{},
	}

	var resp ListOffersResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetOfferRequest struct {
	Zone scw.Zone `json:"-"`
	// OfferID: ID of the researched Offer
	OfferID string `json:"-"`
}

// GetOffer: get offer
//
// Return specific offer for the given ID.
func (s *API) GetOffer(req *GetOfferRequest, opts ...scw.RequestOption) (*Offer, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.OfferID) == "" {
		return nil, errors.New("field OfferID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/offers/" + fmt.Sprint(req.OfferID) + "",
		Headers: http.Header{},
	}

	var resp Offer

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetOptionRequest struct {
	Zone scw.Zone `json:"-"`
	// OptionID: ID of the option
	OptionID string `json:"-"`
}

// GetOption: get option
//
// Return specific option for the given ID.
func (s *API) GetOption(req *GetOptionRequest, opts ...scw.RequestOption) (*Option, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.OptionID) == "" {
		return nil, errors.New("field OptionID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/options/" + fmt.Sprint(req.OptionID) + "",
		Headers: http.Header{},
	}

	var resp Option

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type ListOptionsRequest struct {
	Zone scw.Zone `json:"-"`
	// Page: page number
	Page *int32 `json:"-"`
	// PageSize: number of options per page
	PageSize *uint32 `json:"-"`
	// OfferID: filter options by offer_id
	OfferID *string `json:"-"`
	// Name: filter options by name
	Name *string `json:"-"`
}

// ListOptions: list options
//
// List all options matching with filters.
func (s *API) ListOptions(req *ListOptionsRequest, opts ...scw.RequestOption) (*ListOptionsResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	defaultPageSize, exist := s.client.GetDefaultPageSize()
	if (req.PageSize == nil || *req.PageSize == 0) && exist {
		req.PageSize = &defaultPageSize
	}

	query := url.Values{}
	parameter.AddToQuery(query, "page", req.Page)
	parameter.AddToQuery(query, "page_size", req.PageSize)
	parameter.AddToQuery(query, "offer_id", req.OfferID)
	parameter.AddToQuery(query, "name", req.Name)

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/options",
		Query:   query,
		Headers: http.Header{},
	}

	var resp ListOptionsResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type ListSettingsRequest struct {
	Zone scw.Zone `json:"-"`
	// Page: page number
	Page *int32 `json:"-"`
	// PageSize: set the maximum list size
	PageSize *uint32 `json:"-"`
	// OrderBy: order the response
	//
	// Default value: created_at_asc
	OrderBy ListSettingsRequestOrderBy `json:"-"`
	// ProjectID: ID of the project
	ProjectID *string `json:"-"`
}

// ListSettings: list all settings
//
// Return all settings for a project ID.
func (s *API) ListSettings(req *ListSettingsRequest, opts ...scw.RequestOption) (*ListSettingsResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	defaultPageSize, exist := s.client.GetDefaultPageSize()
	if (req.PageSize == nil || *req.PageSize == 0) && exist {
		req.PageSize = &defaultPageSize
	}

	query := url.Values{}
	parameter.AddToQuery(query, "page", req.Page)
	parameter.AddToQuery(query, "page_size", req.PageSize)
	parameter.AddToQuery(query, "order_by", req.OrderBy)
	parameter.AddToQuery(query, "project_id", req.ProjectID)

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/settings",
		Query:   query,
		Headers: http.Header{},
	}

	var resp ListSettingsResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type UpdateSettingRequest struct {
	Zone scw.Zone `json:"-"`
	// SettingID: ID of the setting
	SettingID string `json:"-"`
	// Enabled: enable/Disable the setting
	Enabled *bool `json:"enabled"`
}

// UpdateSetting: update setting
//
// Update a setting for a project ID (enable or disable).
func (s *API) UpdateSetting(req *UpdateSettingRequest, opts ...scw.RequestOption) (*Setting, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.SettingID) == "" {
		return nil, errors.New("field SettingID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "PATCH",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/settings/" + fmt.Sprint(req.SettingID) + "",
		Headers: http.Header{},
	}

	err = scwReq.SetBody(req)
	if err != nil {
		return nil, err
	}

	var resp Setting

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type ListOSRequest struct {
	Zone scw.Zone `json:"-"`
	// Page: page number
	Page *int32 `json:"-"`
	// PageSize: number of OS per page
	PageSize *uint32 `json:"-"`
	// OfferID: filter OS by offer ID
	OfferID *string `json:"-"`
}

// ListOS: list all available OS that can be install on a elastic metal server
//
// List all available OS that can be install on a elastic metal server.
func (s *API) ListOS(req *ListOSRequest, opts ...scw.RequestOption) (*ListOSResponse, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	defaultPageSize, exist := s.client.GetDefaultPageSize()
	if (req.PageSize == nil || *req.PageSize == 0) && exist {
		req.PageSize = &defaultPageSize
	}

	query := url.Values{}
	parameter.AddToQuery(query, "page", req.Page)
	parameter.AddToQuery(query, "page_size", req.PageSize)
	parameter.AddToQuery(query, "offer_id", req.OfferID)

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/os",
		Query:   query,
		Headers: http.Header{},
	}

	var resp ListOSResponse

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetOSRequest struct {
	Zone scw.Zone `json:"-"`
	// OsID: ID of the OS
	OsID string `json:"-"`
}

// GetOS: get an OS with a given ID
//
// Return specific OS for the given ID.
func (s *API) GetOS(req *GetOSRequest, opts ...scw.RequestOption) (*OS, error) {
	var err error

	if req.Zone == "" {
		defaultZone, _ := s.client.GetDefaultZone()
		req.Zone = defaultZone
	}

	if fmt.Sprint(req.Zone) == "" {
		return nil, errors.New("field Zone cannot be empty in request")
	}

	if fmt.Sprint(req.OsID) == "" {
		return nil, errors.New("field OsID cannot be empty in request")
	}

	scwReq := &scw.ScalewayRequest{
		Method:  "GET",
		Path:    "/baremetal/v1/zones/" + fmt.Sprint(req.Zone) + "/os/" + fmt.Sprint(req.OsID) + "",
		Headers: http.Header{},
	}

	var resp OS

	err = s.client.Do(scwReq, &resp, opts...)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// UnsafeGetTotalCount should not be used
// Internal usage only
func (r *ListServersResponse) UnsafeGetTotalCount() uint32 {
	return r.TotalCount
}

// UnsafeAppend should not be used
// Internal usage only
func (r *ListServersResponse) UnsafeAppend(res interface{}) (uint32, error) {
	results, ok := res.(*ListServersResponse)
	if !ok {
		return 0, errors.New("%T type cannot be appended to type %T", res, r)
	}

	r.Servers = append(r.Servers, results.Servers...)
	r.TotalCount += uint32(len(results.Servers))
	return uint32(len(results.Servers)), nil
}

// UnsafeGetTotalCount should not be used
// Internal usage only
func (r *ListServerEventsResponse) UnsafeGetTotalCount() uint32 {
	return r.TotalCount
}

// UnsafeAppend should not be used
// Internal usage only
func (r *ListServerEventsResponse) UnsafeAppend(res interface{}) (uint32, error) {
	results, ok := res.(*ListServerEventsResponse)
	if !ok {
		return 0, errors.New("%T type cannot be appended to type %T", res, r)
	}

	r.Events = append(r.Events, results.Events...)
	r.TotalCount += uint32(len(results.Events))
	return uint32(len(results.Events)), nil
}

// UnsafeGetTotalCount should not be used
// Internal usage only
func (r *ListOffersResponse) UnsafeGetTotalCount() uint32 {
	return r.TotalCount
}

// UnsafeAppend should not be used
// Internal usage only
func (r *ListOffersResponse) UnsafeAppend(res interface{}) (uint32, error) {
	results, ok := res.(*ListOffersResponse)
	if !ok {
		return 0, errors.New("%T type cannot be appended to type %T", res, r)
	}

	r.Offers = append(r.Offers, results.Offers...)
	r.TotalCount += uint32(len(results.Offers))
	return uint32(len(results.Offers)), nil
}

// UnsafeGetTotalCount should not be used
// Internal usage only
func (r *ListOptionsResponse) UnsafeGetTotalCount() uint32 {
	return r.TotalCount
}

// UnsafeAppend should not be used
// Internal usage only
func (r *ListOptionsResponse) UnsafeAppend(res interface{}) (uint32, error) {
	results, ok := res.(*ListOptionsResponse)
	if !ok {
		return 0, errors.New("%T type cannot be appended to type %T", res, r)
	}

	r.Options = append(r.Options, results.Options...)
	r.TotalCount += uint32(len(results.Options))
	return uint32(len(results.Options)), nil
}

// UnsafeGetTotalCount should not be used
// Internal usage only
func (r *ListSettingsResponse) UnsafeGetTotalCount() uint32 {
	return r.TotalCount
}

// UnsafeAppend should not be used
// Internal usage only
func (r *ListSettingsResponse) UnsafeAppend(res interface{}) (uint32, error) {
	results, ok := res.(*ListSettingsResponse)
	if !ok {
		return 0, errors.New("%T type cannot be appended to type %T", res, r)
	}

	r.Settings = append(r.Settings, results.Settings...)
	r.TotalCount += uint32(len(results.Settings))
	return uint32(len(results.Settings)), nil
}

// UnsafeGetTotalCount should not be used
// Internal usage only
func (r *ListOSResponse) UnsafeGetTotalCount() uint32 {
	return r.TotalCount
}

// UnsafeAppend should not be used
// Internal usage only
func (r *ListOSResponse) UnsafeAppend(res interface{}) (uint32, error) {
	results, ok := res.(*ListOSResponse)
	if !ok {
		return 0, errors.New("%T type cannot be appended to type %T", res, r)
	}

	r.Os = append(r.Os, results.Os...)
	r.TotalCount += uint32(len(results.Os))
	return uint32(len(results.Os)), nil
}
