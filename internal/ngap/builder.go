package ngap

import (
	"encoding/hex"
	"fmt"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/ngap/ngapConvert"
)

func BuildNGSetupRequest(proxy *ProxyRan) ([]byte, error) {
	var pdu ngapType.NGAPPDU
	pdu.Present = ngapType.NGAPPDUPresentInitiatingMessage
	pdu.InitiatingMessage = new(ngapType.InitiatingMessage)

	InitiatingMessage := pdu.InitiatingMessage
	InitiatingMessage.ProcedureCode.Value = ngapType.ProcedureCodeNGSetup
	InitiatingMessage.Criticality.Value = ngapType.CriticalityPresentReject
	InitiatingMessage.Value.Present = ngapType.InitiatingMessagePresentNGSetupRequest
	InitiatingMessage.Value.NGSetupRequest = new(ngapType.NGSetupRequest)

	nGSetupRequest := InitiatingMessage.Value.NGSetupRequest
	nGSetupRequestIEs := &nGSetupRequest.ProtocolIEs

	// 1. GlobalRANNodeID
	ie := ngapType.NGSetupRequestIEs{}
	ie.Id.Value = ngapType.ProtocolIEIDGlobalRANNodeID
	ie.Criticality.Value = ngapType.CriticalityPresentReject
	ie.Value.Present = ngapType.NGSetupRequestIEsPresentGlobalRANNodeID
	ie.Value.GlobalRANNodeID = new(ngapType.GlobalRANNodeID)

	globalRANNodeID := ie.Value.GlobalRANNodeID
	globalRANNodeID.Present = ngapType.GlobalRANNodeIDPresentGlobalGNBID
	globalRANNodeID.GlobalGNBID = new(ngapType.GlobalGNBID)

	globalRANNodeID.GlobalGNBID.PLMNIdentity = ngapConvert.PlmnIdToNgap(*proxy.RanId.PlmnId)
	globalRANNodeID.GlobalGNBID.GNBID.Present = ngapType.GNBIDPresentGNBID
	globalRANNodeID.GlobalGNBID.GNBID.GNBID = &aper.BitString{
		Bytes:     []byte(proxy.RanId.GNbId.GNBValue),
		BitLength: uint64(proxy.RanId.GNbId.BitLength),
	}

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie)

	// 2. RAN Node Name
	ie2 := ngapType.NGSetupRequestIEs{}
	ie2.Id.Value = ngapType.ProtocolIEIDRANNodeName
	ie2.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie2.Value.Present = ngapType.NGSetupRequestIEsPresentRANNodeName
	ie2.Value.RANNodeName = new(ngapType.RANNodeName)
	ie2.Value.RANNodeName.Value = proxy.Name

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie2)

	// 3. SupportedTAList
	ie3 := ngapType.NGSetupRequestIEs{}
	ie3.Id.Value = ngapType.ProtocolIEIDSupportedTAList
	ie3.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie3.Value.Present = ngapType.NGSetupRequestIEsPresentSupportedTAList
	ie3.Value.SupportedTAList = new(ngapType.SupportedTAList)
	supportedTAList := ie3.Value.SupportedTAList

	for _, tai := range proxy.SupportedTAList {
		taiIE := ngapType.SupportedTAItem{}
		var tacBytes []byte

		if len(tai.Tai.Tac) == 3 && (tai.Tai.Tac[0] == 0 || tai.Tai.Tac[1] == 0 || tai.Tai.Tac[2] == 0) {
			tacBytes = []byte(tai.Tai.Tac)
		} else {
			var err error
			tacBytes, err = hex.DecodeString(tai.Tai.Tac)
			if err != nil {
				return nil, fmt.Errorf("[ERROR] invalid TAC format: %v", err)
			}
		}

		if len(tacBytes) == 0 {
			tacBytes = []byte{0x00, 0x00, 0x01}
		} else if len(tacBytes) < 3 {
			paddedTac := make([]byte, 3)
			copy(paddedTac[3-len(tacBytes):], tacBytes)
			tacBytes = paddedTac
		} else if len(tacBytes) > 3 {
			tacBytes = tacBytes[:3]
		}

		taiIE.TAC = ngapType.TAC{
			Value: aper.OctetString(tacBytes),
		}

		taiIE.BroadcastPLMNList.List = make([]ngapType.BroadcastPLMNItem, 1)
		taiIE.BroadcastPLMNList.List[0] = ngapType.BroadcastPLMNItem{
			PLMNIdentity: ngapConvert.PlmnIdToNgap(*tai.Tai.PlmnId),
			TAISliceSupportList: ngapType.SliceSupportList{
				List: make([]ngapType.SliceSupportItem, 0, len(tai.SNssaiList)),
			},
		}

		for _, snssai := range tai.SNssaiList {
			sliceSupportItem := ngapType.SliceSupportItem{
				SNSSAI: ngapType.SNSSAI{
					SST: ngapType.SST{
						Value: aper.OctetString{byte(snssai.Sst)},
					},
				},
			}

			if snssai.Sd != "" {
				sdBytes, err := hex.DecodeString(snssai.Sd)
				if err != nil {
					return nil, fmt.Errorf("[ERROR] invalid SD format: %v", err)
				}
				if len(sdBytes) < 3 {
					paddedSd := make([]byte, 3)
					copy(paddedSd[3-len(sdBytes):], sdBytes)
					sdBytes = paddedSd
				} else if len(sdBytes) > 3 {
					sdBytes = sdBytes[:3]
				}

				sliceSupportItem.SNSSAI.SD = &ngapType.SD{
					Value: aper.OctetString(sdBytes),
				}
			}

			taiIE.BroadcastPLMNList.List[0].TAISliceSupportList.List = append(
				taiIE.BroadcastPLMNList.List[0].TAISliceSupportList.List,
				sliceSupportItem,
			)
		}
		supportedTAList.List = append(supportedTAList.List, taiIE)
	}

	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie3)

	// 4. DefaultPagingDRX
	ie4 := ngapType.NGSetupRequestIEs{}
	ie4.Id.Value = ngapType.ProtocolIEIDDefaultPagingDRX
	ie4.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie4.Value.Present = ngapType.NGSetupRequestIEsPresentDefaultPagingDRX
	ie4.Value.DefaultPagingDRX = new(ngapType.PagingDRX)

	switch proxy.DefaultPagingDRX {
	case "v128":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV128}
	case "v64":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV64}
	case "v32":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV32}
	case "v256":
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV256}
	default:
		ie4.Value.DefaultPagingDRX = &ngapType.PagingDRX{Value: ngapType.PagingDRXPresentV128}
	}
	nGSetupRequestIEs.List = append(nGSetupRequestIEs.List, ie4)

	return ngap.Encoder(pdu)
}