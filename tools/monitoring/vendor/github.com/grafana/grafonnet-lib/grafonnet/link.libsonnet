{
  /**
   * Creates [links](https://grafana.com/docs/grafana/latest/linking/linking-overview/) to navigate to other dashboards.
   *
   * @param title Human-readable label for the link.
   * @param tags Limits the linked dashboards to only the ones with the corresponding tags. Otherwise, Grafana includes links to all other dashboards.
   * @param asDropdown (default: `true`) Whether to use a dropdown (with an optional title). If `false`, displays the dashboard links side by side across the top of dashboard.
   * @param includeVars (default: `false`) Whether to include template variables currently used as query parameters in the link. Any matching templates in the linked dashboard are set to the values from the link
   * @param keepTime (default: `false`) Whether to include the current dashboard time range in the link (e.g. from=now-3h&to=now)
   * @param icon (default: `'external link'`) Icon displayed with the link.
   * @param url (default: `''`) URL of the link
   * @param targetBlank (default: `false`) Whether the link will open in a new window.
   * @param type (default: `'dashboards'`)
   *
   * @name link.dashboards
   */
  dashboards(
    title,
    tags,
    asDropdown=true,
    includeVars=false,
    keepTime=false,
    icon='external link',
    url='',
    targetBlank=false,
    type='dashboards',
  )::
    {
      asDropdown: asDropdown,
      icon: icon,
      includeVars: includeVars,
      keepTime: keepTime,
      tags: tags,
      title: title,
      type: type,
      url: url,
      targetBlank: targetBlank,
    },
}
