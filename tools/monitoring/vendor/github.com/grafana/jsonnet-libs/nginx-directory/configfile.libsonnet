function(services) {
  local buildHeaders(service) =
    (
      if std.length(service.custom) > 0
      then std.join('\n', service.custom) + '\n'
      else ''
    )
    + (
      if service.redirect
      then |||
        return 302 %(url)s;
      ||| % service
      else |||
        proxy_pass      %(url)s$2$is_args$args;
        proxy_set_header    Host $host;
        proxy_set_header    X-Real-IP $remote_addr;
        proxy_set_header    X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header    X-Forwarded-Proto $scheme;
        proxy_set_header    X-Forwarded-Host $http_host;
        proxy_read_timeout  %(read_timeout)s;
        proxy_send_timeout  %(send_timeout)s;
      ||| % (service)
    )
    + (
      if service.allowWebsockets
      then |||
        # Allow websocket connections https://www.nginx.com/blog/websocket-nginx/
        proxy_set_header    Upgrade $http_upgrade;
        proxy_set_header    Connection "Upgrade";
      |||
      else ''
    )
    + (
      if service.subfilter
      then |||
        proxy_set_header Accept-Encoding "";
        sub_filter 'href="/' 'href="/%(path)s/';
        sub_filter 'src="/' 'src="/%(path)s/';
        sub_filter 'action="/' 'action="/%(path)s/';
        sub_filter 'endpoint:"/' 'endpoint:"/%(path)s/';  # for XHRs.
        sub_filter 'href:"/v1/' 'href:"/%(path)s/v1/';
        sub_filter_once off;
        sub_filter_types %(rendered_subfilter_content_types)s;
        proxy_redirect   "/" "/%(path)s/";
      ||| % (service { rendered_subfilter_content_types: std.join(' ', self.subfilter_content_types) })
      else ''
    ),

  local buildLocation(service) =
    |||
      location ~ ^/%(path)s(/?)(.*)$ {
    ||| % service +
    buildHeaders(service) +
    |||
      }
    |||,

  local servicesByWeight(services) =
    std.sort(services, function(s) if std.objectHas(s, 'weight') then s.weight else 0),

  location_stanzas: [
    buildLocation(service)
    for service in std.uniq(servicesByWeight(services), function(s) s.url)
  ],
  locations: std.join('\n', self.location_stanzas),
}
