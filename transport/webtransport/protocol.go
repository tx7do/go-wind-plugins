package webtransport

const settingsEnableWebtransport = 0x2b603742

const protocolHeader = "webtransport"

const (
	webTransportFrameType     = 0x41
	webTransportUniStreamType = 0x54
)

const (
	webTransportDraftOfferHeaderKey = "Sec-Webtransport-Http3-Draft02"
	webTransportDraftHeaderKey      = "Sec-Webtransport-Http3-Draft"
	webTransportDraftHeaderValue    = "draft02"
)

const (
	// https://datatracker.ietf.org/doc/html/draft-ietf-webtrans-http3/
	alpnQuicTransport = "h3"

	// https://tools.ietf.org/html/draft-vvv-webtransport-quic-02#section-3.2
	maxClientIndicationLength = 65535
)
