{
  new(name, url, type, default=false):: {
    name: name,
    type: type,
    access: 'proxy',
    url: url,
    isDefault: default,
    version: 1,
    editable: false,
  },
  // `legacy` causes a datasource to use the old, less secure `basicAuthPassword` element
  // rather than the more up-to-date `secureJsonData.basicAuthPassword` which was introduced in Grafana 6.2
  withBasicAuth(username, password, legacy=false):: {
    basicAuth: true,
    basicAuthUser: username,
    [if legacy then 'basicAuthPassword']: password,
    [if !legacy then 'secureJsonData']+: {
      basicAuthPassword: password,
    },
  },
  withJsonData(data):: {
    jsonData+: data,
  },
  withSecureJsonData(data):: {
    secureJsonData+: data,
  },
  withHttpMethod(httpMethod):: self.withJsonData({ httpMethod: httpMethod }),
}
