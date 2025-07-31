package ue

type Context struct {
	amf     interface{} // Will be *amf.Amf but avoiding circular import
	amfUeId int64
	gnb     interface{} // Will be *gnb.Gnb but avoiding circular import
	gnbUeId int64
	lbId    int64
}

func New(amf interface{}, gnb interface{}, gnbUeId, lbId int64) *Context {
	return &Context{
		amf:     amf,
		gnb:     gnb,
		gnbUeId: gnbUeId,
		lbId:    lbId,
	}
}

func (ue *Context) SetAmfUeId(id int64) {
	ue.amfUeId = id
}

func (ue *Context) GetAmfUeId() int64 {
	return ue.amfUeId
}

func (ue *Context) GetGnbUeId() int64 {
	return ue.gnbUeId
}

func (ue *Context) GetLbId() int64 {
	return ue.lbId
}

func (ue *Context) GetAmf() interface{} {
	return ue.amf
}

func (ue *Context) GetGnb() interface{} {
	return ue.gnb
}

func (ue *Context) UpdateIds(newGnbUeId, newAmfUeId int64) {
	updated := false
	if ue.gnbUeId != newGnbUeId {
		ue.gnbUeId = newGnbUeId
		updated = true
	}
	if ue.amfUeId != newAmfUeId {
		ue.amfUeId = newAmfUeId
		updated = true
	}
	if updated {
		ue.lbId++
	}
}
