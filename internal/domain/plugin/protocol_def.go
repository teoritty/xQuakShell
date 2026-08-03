package plugin

// ProtocolDef is a cached, indexed view of a connection protocol contribution.
type ProtocolDef struct {
	Protocol   ConnectionProtocolContribution
	Groups     []FieldGroup
	FlatFields map[string]*FieldDef
	FieldIDs   map[string]bool
}

// GetFieldIDs returns declared field IDs for capability checks.
func (p *ProtocolDef) GetFieldIDs() map[string]bool {
	if p == nil {
		return nil
	}
	return p.FieldIDs
}

// GetFlatFields returns all declared fields as a slice.
func (p *ProtocolDef) GetFlatFields() []FieldDef {
	if p == nil {
		return nil
	}
	fields := make([]FieldDef, 0, len(p.FlatFields))
	for _, f := range p.FlatFields {
		fields = append(fields, *f)
	}
	return fields
}

// BuildProtocolDef indexes fields for a single protocol contribution.
func BuildProtocolDef(proto ConnectionProtocolContribution) *ProtocolDef {
	flatFields := make(map[string]*FieldDef)
	fieldIDs := make(map[string]bool)

	for gi := range proto.Fields {
		group := &proto.Fields[gi]
		for fi := range group.Fields {
			field := &group.Fields[fi]
			flatFields[field.ID] = field
			fieldIDs[field.ID] = true
		}
	}

	return &ProtocolDef{
		Protocol:   proto,
		Groups:     proto.Fields,
		FlatFields: flatFields,
		FieldIDs:   fieldIDs,
	}
}

// BuildProtocolDefs indexes all contributed connection protocols.
func BuildProtocolDefs(m *Manifest) map[string]*ProtocolDef {
	out := make(map[string]*ProtocolDef, len(m.Contributions.ConnectionProtocols))
	for _, proto := range m.Contributions.ConnectionProtocols {
		out[proto.ID] = BuildProtocolDef(proto)
	}
	return out
}
