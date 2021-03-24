import React from "react";

import Highcharts from 'highcharts'
import HighchartsReact from 'highcharts-react-official'
require("highcharts/modules/exporting")(Highcharts);


function ExportData(props) {
  const cc = React.useRef(null)
  const options = {
    exporting: {buttons: { contextButton: {menuItems:
            ["downloadPNG", "downloadJPEG", "downloadPDF"]
    }}}}

  const exportGraph = () => {
    // chart.exportChart()
    console.log(cc.current.chart)
    cc.current.chart.exportChart()
  }

  // vanilla handler solution
  const exportCanvasAsPNG = () => {
    const canvasElement = document.querySelector('.flamegraph-canvas');
    const MIME_TYPE = "image/png";
    const imgURL = canvasElement.toDataURL(MIME_TYPE);
    const dlLink = document.createElement('a');

    dlLink.download = `${Date.now()}file`;
    dlLink.href = imgURL;
    dlLink.dataset.downloadurl = [MIME_TYPE, dlLink.download, dlLink.href].join(':');

    document.body.appendChild(dlLink);
    dlLink.click();
    document.body.removeChild(dlLink);
  }

  // Highcharts WIP
  return (
    <div style={{display: 'flex'}}>
      <HighchartsReact
        highcharts={Highcharts} options={options} ref={cc} />

      {/*<button onClick={exportGraph}>Export</button>*/}
      <button onClick={exportCanvasAsPNG}>Export</button>
    </div>
  );
}

export default ExportData;
