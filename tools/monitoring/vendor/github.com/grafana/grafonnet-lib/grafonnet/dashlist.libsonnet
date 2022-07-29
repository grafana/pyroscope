{
  /**
   * Creates a [dashlist panel](https://grafana.com/docs/grafana/latest/panels/visualizations/dashboard-list-panel/).
   * It requires the dashlist panel plugin in grafana, which is built-in.
   *
   * @name dashlist.new
   *
   * @param title The title of the dashlist panel.
   * @param description (optional) Description of the panel
   * @param query (optional) Query to search by
   * @param tags (optional) Array of tag(s) to search by
   * @param recent (default `true`) Displays recently viewed dashboards
   * @param search (default `false`) Description of the panel
   * @param starred (default `false`) Displays starred dashboards
   * @param headings (default `true`) Chosen list selection(starred, recently Viewed, search) is shown as a heading
   * @param limit (default `10`) Set maximum items in a list
   * @return A json that represents a dashlist panel
   */
  new(
    title,
    description=null,
    query=null,
    tags=[],
    recent=true,
    search=false,
    starred=false,
    headings=true,
    limit=10,
  ):: {
    type: 'dashlist',
    title: title,
    query: if query != null then query else '',
    tags: tags,
    recent: recent,
    search: search,
    starred: starred,
    headings: headings,
    limit: limit,
    [if description != null then 'description']: description,
  },
}
