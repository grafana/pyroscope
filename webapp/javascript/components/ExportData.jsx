import React from "react";

import Highcharts from 'highcharts'
import HighchartsReact from 'highcharts-react-official'

const options = {
  title: {
    text: 'My Chart'
  },
  series: [{
    data: [1,2,3]
  }]
}

function ExportData() {
  return (
    <div onClick={() => console.log('this')} style={{background: 'lime'}}>
      Open Example
    </div>
  );
}

export default ExportData;


// <HighchartsReact highcharts={Highcharts} options={options}/>