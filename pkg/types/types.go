package types

import (
	"log"
	"net"
	"sync"

	"github.com/free5gc/openapi/models"
	"github.com/sirupsen/logrus"
)

// AmfRan represents a RAN node in the network, specifically for Load balancer.
type AmfRan struct {
	RanPresent      int
	RanId           *models.GlobalRanNodeId
	Name            string
	AnType          models.AccessType
	Conn            net.Conn
	SupportedTAList []SupportedTAI
	RanUeList       sync.Map // RanUeNgapId as key
	Log             *logrus.Entry
}

// ProxyContext represents the context for a proxy work with gNB RAN nodes.
type ProxyContext struct {
	Name             string
	ServedGUAMIList  []models.Guami
	RelativeCapacity int64
	NgapIPList       []string
	NgapPort         int
	PlmnSupportList  []PlmnSupportItem
}

// ProxyRan represents a proxy as a gNB RAN node for AMF in the network.
type ProxyRan struct {
	Name               string
	NRCellIdentifier   string
	PlmnRANSupportList []PlmnSupportItem
	idlength           int64
	tac                int64
	RanPresent         int
	RanId              *models.GlobalRanNodeId
	SupportedTAList    []SupportedTAI
	DefaultPagingDRX   string
	UERetentionInfo    string
	AnType             models.AccessType
	IPaddress          string
	Port               int
	Log                *log.Logger
}

type SupportedTAI struct {
	Tai        models.Tai
	SNssaiList []models.Snssai
}

type PlmnSupportItem struct {
	PlmnId     *models.PlmnId
	SNssaiList []models.Snssai
}

type AMFInfo struct {
	PodName      string
	NodeIP       string
	InternalPort int32
	NodePort     int32
}
