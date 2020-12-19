// Note: Bind this to the component that calls it

export function buildRenderURL() {
  let width = document.body.clientWidth - 30;
  let url = `/render?from=${encodeURIComponent(this.props.from)}&until=${encodeURIComponent(this.props.until)}&width=${width}`;
  let nameLabel = this.props.labels.find(x => x.name == "__name__");
  if (nameLabel) {
    url += "&name=" + nameLabel.value + "{";
  } else {
    url += "&name=unknown{";
  }

  url += this.props.labels.filter(x => x.name != "__name__").map(x => `${x.name}=${x.value}`).join(",");
  url += "}";
  if (this.props.refreshToken) {
    url += `&refreshToken=${this.props.refreshToken}`
  }
  url += `&max-nodes=${this.props.maxNodes}`
  return url;
}

// Note: Bind this to the component that calls it

export function fetchJSON(url) {
  console.log('fetching json', url);
  if (this.currentJSONController) {
    this.currentJSONController.abort();
  }

  this.currentJSONController = new AbortController();
  fetch(url, { signal: this.currentJSONController.signal })
    .then((response) => {
      return response.json()
    })
    .then((data) => {
      console.log('data:', data);
      console.log('this: ', this);
      console.dir(this);
      this.props.actions.receiveJSON(data)
    })
    .finally();
}