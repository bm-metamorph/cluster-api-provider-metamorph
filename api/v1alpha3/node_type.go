package v1alpha3

type Node struct {
	Name        string `yaml:"name" json:"name"`
	ISOURL      string `yaml:"isoUrl" json:"isoUrl"`
	ISOChecksum string `yaml:"isoChecksum" json:"isoChecksum"`
	//ImageURL    string
	//ChecksumURL           string
	//ImageReadilyAvailable bool
	//OamIP                 string
	//OamGateway            string
	//NameServers           []NameServer  `json:"NameServers"`
	OsDisk       string        `yaml:"osDisk" json:"osDisk"`
	Partitions   []Partition   `yaml:"partitions" json:"partitions"`
	GrubConfig   string        `yaml:"grubConfig" json:"grubConfig"`
	KvmPolicy    KvmPolicy     `yaml:"kvmPolicy" json:"kvmPolicy"`
	SSHPubKeys   []SSHPubKey   `yaml:"sshPubKeys" json:"sshPubKeys"`
	IPMIIP       string        `yaml:"ipmiIp" json:"ipmiIp"`
	IPMIUser     string        `yaml:"ipmiUser" json:"ipmiUser"`
	IPMIPassword string        `yaml:"ipmiPassword" json:"ipmiPassword"`
	Vendor       string        `yaml:"vendor" json:"vendor"`
	ServerModel  string        `yaml:"model" json:"model"`
	VirtualDisks []VirtualDisk `yaml:"virtualDisks" json:"virtualDisks"`
	State        string        `yaml:"State,omitempty" json:"State,omitempty"`
	//ProvisioningIP        string
	//ProvisionerPort       int
	//HTTPPort              int
	BootActions   []BootAction `yaml:"bootActions" json:"bootActions"`
	NetworkConfig string       `yaml:"networkConfig" json:"networkConfig"`
	RAID_reset    bool         `yaml:"raidReset" json:"raidReset"`
	//RedfishManagerID      string
	//RedfishSystemID       string
	//RedfishVersion        string
	Domain               string     `yaml:"domain" json:"domain"`
	Firmwares            []Firmware `yaml:"firmwares" json:"firmwares"`
	AllowFirmwareUpgrade bool       `yaml:"allowFirwareUpgrade" json:"allowFirwareUpgrade"`
	Plugins              Plugins    `yaml:"plugins" json:"plugins"`
	CloudInit            string     `yaml:"cloudInit" json:"cloudInit"`
}

type Plugins struct {
	APIs []API `yaml:"apis" json:"apis"`
}

type API struct {
	//PluginsID uint
	Name   string `yaml:"name" json:"name"`
	Plugin string `yaml:"plugin" json:"plugin"`
}

type Firmware struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
	URL     string `yaml:"url" json:"url"`
}
type BootAction struct {
	Name     string `yaml:"name" json:"name"`
	Location string `yaml:"location" json:"location"`
	Priority uint   `yaml:"priority" json:"priority"`
	Control  string `yaml:"control" json:"control"`
	Args     string `yaml:"args,omitempty" json:"args,omitempty"`
}

type NameServer struct {
	NameServer string `yaml:"NameServer" json:"NameServer"`
}

type Partition struct {
	Name       string     `yaml:"name" json:"name"`
	Size       string     `yaml:"size" json:"size"`
	Bootable   bool       `yaml:"bootable,omitempty" json:"bootable,omitempty"`
	Primary    bool       `yaml:"primary,omitempty" json:"primary,omitempty"`
	Filesystem Filesystem `yaml:"filesystem" json:"filesystem"`
}

type Filesystem struct {
	//PartitionID  uint
	Mountpoint   string `yaml:"mountpoint" json:"mountpoint"`
	Fstype       string `yaml:"fstype" json:"fstype"`
	MountOptions string `yaml:"mount-options" json:"mount-options"`
}

type KvmPolicy struct {
	CpuAllocation     string `yaml:"cpuAllocation" json:"cpuAllocation"`
	CpuPinning        string `yaml:"cpuHyperthreading" json:"cpuHyperthreading"`
	CpuHyperthreading string `yaml:"cpuPinning" json:"cpuPinning"`
}

type SSHPubKey struct {
	SSHPubKey string `yaml:"sshPubKey" json:"sshPubKey"`
}

type BondInterface struct {
	BondInterface string
}

type BondParameter struct {
	Key   string
	Value string
}

type VirtualDisk struct {
	DiskName       string         `yaml:"diskName" json:"diskName"`
	RaidType       int            `yaml:"raidType" json:"raidType"`
	RaidController string         `yaml:"raidController" json:"raidController"`
	PhysicalDisks  []PhysicalDisk `yaml:"physicalDisks" json:"physicalDisks"`
}

type PhysicalDisk struct {
	VirtualDiskID uint   `yaml:"virtualDiskID" json:"virtualDiskID"`
	PhysicalDisk  string `yaml:"physicalDisk" json:"physicalDisk"`
}
