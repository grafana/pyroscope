{
  new(
    name,
    type,
    uid,
    org_id,
    settings,
    is_default=false,
    send_reminders=true,
    frequency='1h',
    disable_resolve_message=false
  ):: {
    name: name,
    type: type,
    uid: uid,
    org_id: org_id,
    is_default: is_default,
    send_reminders: send_reminders,
    frequency: frequency,
    disable_resolve_message: disable_resolve_message,
    settings: settings,
  },
}
