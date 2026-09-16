package platform

import _ "embed"

// ServiceName is the systemd user unit the daemon runs as (R4.5).
const ServiceName = "clockwork-orange.service"

// UnitFile is the systemd unit Install writes (R4.5). It is embedded so the
// installed binary needs no source checkout, unlike the Python which copied
// the unit from the repository.
//
//go:embed clockwork-orange.service
var UnitFile []byte
