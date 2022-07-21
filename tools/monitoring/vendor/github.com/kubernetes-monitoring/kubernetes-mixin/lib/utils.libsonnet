{
  mapRuleGroups(f): {
    groups: [
      group {
        rules: [
          f(rule)
          for rule in super.rules
        ],
      }
      for group in super.groups
    ],
  },

  humanizeSeconds(s)::
    if s > 60 * 60 * 24
    then '%.1f days' % (s / 60 / 60 / 24)
    else '%.1f hours' % (s / 60 / 60),
}
