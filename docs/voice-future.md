# Voice Integration Notes

Voice is scaffolded only in this MVP.

## Implemented Now

- Protocol events exist for `call_invite`, `call_accept`, `call_reject`, `call_hangup`, and `mute_state_changed`.
- The client has call UI state for target user, ringing/active state, and mute state.
- The server has a `CallManager` interface and a no-op implementation.
- The database includes a `future_calls` placeholder table.

## Intended Next Step

- Replace `internal/server/call.NoopManager` with a signaling service backed by Redis pubsub or in-process routing.
- Add SDP offer/answer and ICE candidate events to the shared protocol.
- Create a `Pion` integration package for media engine setup and peer connection lifecycle.
- Extend the TUI to show incoming call prompts, active participants, device state, and reconnection state.

## Integration Boundary

Keep WebRTC media setup out of the main chat hub. The hub should route signaling envelopes and own authorization, while a dedicated call service manages:

- peer session state
- call invitations and timeouts
- mute/deafen metadata
- future TURN/STUN config
