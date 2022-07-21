# Docs

* [dashboard](#dashboard)
* [panel](#panel)
  * [gauge.new](#panelGaugenew)
  * [graph.new](#panelGraphnew)
  * [row.new](#panelRownew)
  * [stat.new](#panelStatnew)
  * [table.new](#panelTablenew)
  * [text.new](#panelTextnew)
* [target](#target)
  * [prometheus.new](#targetPrometheusnew)
* [template](#template)
  * [custom.new](#templateCustomnew)
  * [datasource.new](#templateDatasourcenew)
  * [query.new](#templateQuerynew)

## dashboard



### dashboard.new

Instantiate a dashboard.

* **description**: (type: string, default: `null`)
  
* **editable**: (type: boolean, default: `true`)
  
* **graphTooltip**: (type: integer, default: `0`)
  
* **refresh**: (type: string, default: `null`)
  
* **schemaVersion**: (type: integer, default: `25`)
  
* **style**: (type: string, default: `"dark"`)
  
* **tags**: (type: array, default: `[]`)
  
* **timezone**: (type: string, default: `null`)
  
* **title**: (type: string, default: `null`)
  
* **uid**: (type: string, default: `null`)
  

#### #setTime

* **from**: (type: string, default: `"now-6h"`)
  
* **to**: (type: string, default: `"now"`)
  
#### #setTimepicker

* **hidden**: (type: boolean, default: `false`)
  
* **refreshIntervals**: (type: array, default: `["5s","10s","30s","1m","5m","15m","30m","1h","2h","1d"]`)
  

#### #addAnnotation

* **builtIn**: (type: integer, default: `0`)
  
* **datasource**: (type: string, default: `"default"`)
  
* **enable**: (type: boolean, default: `true`)
  
* **hide**: (type: boolean, default: `false`)
  
* **iconColor**: (type: string, default: `null`)
  
* **name**: (type: string, default: `null`)
  
* **rawQuery**: (type: string, default: `null`)
  
* **showIn**: (type: integer, default: `0`)
  
#### #addTemplate

* **template**: (type: object)
  


## panel



### panel.gauge.new



* **datasource**: (type: string, default: `"default"`)
  
* **description**: (type: string, default: `null`)
  
* **repeat**: (type: string, default: `null`)
  
* **repeatDirection**: (type: string, default: `null`)
  
* **title**: (type: string, default: `null`)
  
* **transparent**: (type: boolean, default: `false`)
  

#### #setFieldConfig

* **max**: (type: integer, default: `null`)
  
* **min**: (type: integer, default: `null`)
  
* **thresholdMode**: (type: string, default: `"absolute"`)
  
* **unit**: (type: string, default: `null`)
  
#### #setGridPos

* **h**: (type: integer, default: `8`)
  Panel height.
* **w**: (type: integer, default: `12`)
  Panel width.
* **x**: (type: integer, default: `null`)
  Panel x position.
* **y**: (type: integer, default: `null`)
  Panel y position.
#### #setOptions

* **calcs**: (type: array, default: `["mean"]`)
  
* **fields**: (type: string, default: `null`)
  
* **orientation**: (type: string, default: `"auto"`)
  
* **showThresholdLabels**: (type: boolean, default: `false`)
  
* **showThresholdMarkers**: (type: boolean, default: `true`)
  
* **values**: (type: boolean, default: `false`)
  

#### #addDataLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addPanelLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addMapping

* **from**: (type: string, default: `null`)
  
* **id**: (type: integer, default: `null`)
  
* **operator**: (type: string, default: `null`)
  
* **text**: (type: string, default: `null`)
  
* **to**: (type: string, default: `null`)
  
* **type**: (type: integer, default: `null`)
  
* **value**: (type: string, default: `null`)
  
#### #addOverride

* **matcher**: (type: oject, default: `null`)
  
* **properties**: (type: array, default: `null`)
  
#### #addThresholdStep

* **color**: (type: string, default: `null`)
  
* **value**: (type: integer, default: `null`)
  
#### #addTarget

* **target**: (type: object)
  


### panel.graph.new



* **bars**: (type: boolean, default: `false`)
  Display values as a bar chart.
* **dashLength**: (type: integer, default: `10`)
  Dashed line length.
* **dashes**: (type: boolean, default: `false`)
  Show line with dashes.
* **datasource**: (type: string, default: `"default"`)
  
* **decimals**: (type: integer, default: `null`)
  Controls how many decimals are displayed for legend values and
  graph hover tooltips.
* **description**: (type: string, default: `null`)
  
* **fill**: (type: integer, default: `1`)
  Amount of color fill for a series. Expects a value between 0 and 1.
* **fillGradient**: (type: integer, default: `0`)
  Degree of gradient on the area fill. 0 is no gradient, 10 is a
  steep gradient.
* **hiddenSeries**: (type: boolean, default: `false`)
  Hide the series.
* **lines**: (type: boolean, default: `true`)
  Display values as a line graph.
* **linewidth**: (type: integer, default: `1`)
  The width of the line for a series.
* **nullPointMode**: (type: string, default: `"null"`)
  How null values are displayed.
  * 'null' - If there is a gap in the series, meaning a null value,
    then the line in the graph will be broken and show the gap.
  * 'null as zero' - If there is a gap in the series, meaning a null
    value, then it will be displayed as a zero value in the graph
    panel.
  * 'connected' - If there is a gap in the series, meaning a null
    value or values, then the line will skip the gap and connect to the
    next non-null value.
* **percentage**: (type: boolean, default: `false`)
  Available when `stack` is true. Each series is drawn as a percentage
  of the total of all series.
* **pointradius**: (type: integer, default: `null`)
  Controls how large the points are.
* **points**: (type: boolean, default: `false`)
  Display points for values.
* **repeat**: (type: string, default: `null`)
  
* **repeatDirection**: (type: string, default: `null`)
  
* **spaceLength**: (type: integer, default: `10`)
  Dashed line spacing when `dashes` is true.
* **stack**: (type: boolean, default: `false`)
  Each series is stacked on top of another.
* **steppedLine**: (type: boolean, default: `false`)
  Draws adjacent points as staircase.
* **timeFrom**: (type: string, default: `null`)
  
* **timeShift**: (type: string, default: `null`)
  
* **title**: (type: string, default: `null`)
  
* **transparent**: (type: boolean, default: `false`)
  

#### #setGridPos

* **h**: (type: integer, default: `8`)
  Panel height.
* **w**: (type: integer, default: `12`)
  Panel width.
* **x**: (type: integer, default: `null`)
  Panel x position.
* **y**: (type: integer, default: `null`)
  Panel y position.
#### #setLegend

* **alignAsTable**: (type: boolean, default: `null`)
  Whether to display legend in table.
* **avg**: (type: boolean, default: `false`)
  Average of all values returned from the metric query.
* **current**: (type: boolean, default: `false`)
  Last value returned from the metric query.
* **max**: (type: boolean, default: `false`)
  Maximum of all values returned from the metric query.
* **min**: (type: boolean, default: `false`)
  Minimum of all values returned from the metric query.
* **rightSide**: (type: boolean, default: `false`)
  Display legend to the right.
* **show**: (type: boolean, default: `true`)
  Show or hide the legend.
* **sideWidth**: (type: integer, default: `null`)
  Available when `rightSide` is true. The minimum width for the legend in
  pixels.
* **total**: (type: boolean, default: `false`)
  Sum of all values returned from the metric query.
* **values**: (type: boolean, default: `true`)
  
#### #setThresholds

* **thresholdMode**: (type: string, default: `"absolute"`)
  
#### #setTooltip

* **shared**: (type: boolean, default: `true`)
  * true - The hover tooltip shows all series in the graph.
    Grafana highlights the series that you are hovering over in
    bold in the series list in the tooltip.
  * false - The hover tooltip shows only a single series, the one
    that you are hovering over on the graph.
* **sort**: (type: integer, default: `2`)
  * 0 (none) - The order of the series in the tooltip is
    determined by the sort order in your query. For example, they
    could be alphabetically sorted by series name.
  * 1 (increasing) - The series in the hover tooltip are sorted
    by value and in increasing order, with the lowest value at the
    top of the list.
  * 2 (decreasing) - The series in the hover tooltip are sorted
    by value and in decreasing order, with the highest value at the
    top of the list.
#### #setXaxis

* **buckets**: (type: string, default: `null`)
  
* **mode**: (type: string, default: `"time"`)
  The display mode completely changes the visualization of the
  graph panel. It’s like three panels in one. The main mode is
  the time series mode with time on the X-axis. The other two
  modes are a basic bar chart mode with series on the X-axis
  instead of time and a histogram mode.
  * 'time' - The X-axis represents time and that the data is
    grouped by time (for example, by hour, or by minute).
  * 'series' - The data is grouped by series and not by time. The
    Y-axis still represents the value.
  * 'histogram' - Converts the graph into a histogram. A histogram
    is a kind of bar chart that groups numbers into ranges, often
    called buckets or bins. Taller bars show that more data falls
    in that range.
* **name**: (type: string, default: `null`)
  
* **show**: (type: boolean, default: `true`)
  Show or hide the axis.
#### #setYaxis

* **align**: (type: boolean, default: `false`)
  Align left and right Y-axes by value.
* **alignLevel**: (type: integer, default: `0`)
  Available when align is true. Value to use for alignment of
  left and right Y-axes, starting from Y=0.

#### #addDataLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addPanelLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addOverride

* **matcher**: (type: oject, default: `null`)
  
* **properties**: (type: array, default: `null`)
  
#### #addSeriesOverride

* **alias**: (type: string, default: `null`)
  Alias or regex matching the series you'd like to target.
* **bars**: (type: boolean, default: `null`)
  
* **color**: (type: string, default: `null`)
  
* **dashLength**: (type: integer, default: `null`)
  
* **dashes**: (type: boolean, default: `null`)
  
* **fill**: (type: integer, default: `null`)
  
* **fillBelowTo**: (type: string, default: `null`)
  
* **fillGradient**: (type: integer, default: `null`)
  
* **hiddenSeries**: (type: boolean, default: `null`)
  
* **hideTooltip**: (type: boolean, default: `null`)
  
* **legend**: (type: boolean, default: `null`)
  
* **lines**: (type: boolean, default: `null`)
  
* **linewidth**: (type: integer, default: `null`)
  
* **nullPointMode**: (type: string, default: `null`)
  
* **pointradius**: (type: integer, default: `null`)
  
* **points**: (type: boolean, default: `null`)
  
* **spaceLength**: (type: integer, default: `null`)
  
* **stack**: (type: integer, default: `null`)
  
* **steppedLine**: (type: boolean, default: `null`)
  
* **transform**: (type: string, default: `null`)
  
* **yaxis**: (type: integer, default: `null`)
  
* **zindex**: (type: integer, default: `null`)
  
#### #addThresholdStep

* **color**: (type: string, default: `null`)
  
* **value**: (type: integer, default: `null`)
  
#### #addTarget

* **target**: (type: object)
  
#### #addYaxis

* **decimals**: (type: integer, default: `null`)
  Defines how many decimals are displayed for Y value.
* **format**: (type: string, default: `"short"`)
  The display unit for the Y value.
* **label**: (type: string, default: `null`)
  The Y axis label.
* **logBase**: (type: integer, default: `1`)
  The scale to use for the Y value - linear, or logarithmic.
  * 1 - linear
  * 2 - log (base 2)
  * 10 - log (base 10)
  * 32 - log (base 32)
  * 1024 - log (base 1024)
* **max**: (type: integer, default: `null`)
  The maximum Y value.
* **min**: (type: integer, default: `null`)
  The minimum Y value.
* **show**: (type: boolean, default: `true`)
  Show or hide the axis.


### panel.row.new



* **collapse**: (type: boolean, default: `true`)
  
* **collapsed**: (type: boolean, default: `true`)
  
* **datasource**: (type: string, default: `null`)
  
* **repeat**: (type: string, default: `null`)
  
* **repeatIteration**: (type: string, default: `null`)
  
* **showTitle**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **titleSize**: (type: string, default: `"h6"`)
  

#### #setGridPos

* **h**: (type: integer, default: `8`)
  Panel height.
* **w**: (type: integer, default: `12`)
  Panel width.
* **x**: (type: integer, default: `null`)
  Panel x position.
* **y**: (type: integer, default: `null`)
  Panel y position.

#### #addPanel

* **panel**: (type: object)
  


### panel.stat.new



* **datasource**: (type: string, default: `"default"`)
  
* **description**: (type: string, default: `null`)
  
* **repeat**: (type: string, default: `null`)
  
* **repeatDirection**: (type: string, default: `null`)
  
* **title**: (type: string, default: `null`)
  
* **transparent**: (type: boolean, default: `false`)
  

#### #setFieldConfig

* **max**: (type: integer, default: `null`)
  
* **min**: (type: integer, default: `null`)
  
* **thresholdMode**: (type: string, default: `"absolute"`)
  
* **unit**: (type: string, default: `null`)
  
#### #setGridPos

* **h**: (type: integer, default: `8`)
  Panel height.
* **w**: (type: integer, default: `12`)
  Panel width.
* **x**: (type: integer, default: `null`)
  Panel x position.
* **y**: (type: integer, default: `null`)
  Panel y position.
#### #setOptions

* **calcs**: (type: array, default: `["mean"]`)
  
* **colorMode**: (type: string, default: `"value"`)
  
* **fields**: (type: string, default: `null`)
  
* **graphMode**: (type: string, default: `"none"`)
  
* **justifyMode**: (type: string, default: `"auto"`)
  
* **orientation**: (type: string, default: `"auto"`)
  
* **textMode**: (type: string, default: `"auto"`)
  
* **values**: (type: boolean, default: `false`)
  

#### #addDataLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addPanelLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addMapping

* **from**: (type: string, default: `null`)
  
* **id**: (type: integer, default: `null`)
  
* **operator**: (type: string, default: `null`)
  
* **text**: (type: string, default: `null`)
  
* **to**: (type: string, default: `null`)
  
* **type**: (type: integer, default: `null`)
  
* **value**: (type: string, default: `null`)
  
#### #addOverride

* **matcher**: (type: oject, default: `null`)
  
* **properties**: (type: array, default: `null`)
  
#### #addThresholdStep

* **color**: (type: string, default: `null`)
  
* **value**: (type: integer, default: `null`)
  
#### #addTarget

* **target**: (type: object)
  


### panel.table.new



* **datasource**: (type: string, default: `"default"`)
  
* **description**: (type: string, default: `null`)
  
* **repeat**: (type: string, default: `null`)
  
* **repeatDirection**: (type: string, default: `null`)
  
* **title**: (type: string, default: `null`)
  
* **transparent**: (type: boolean, default: `false`)
  

#### #setFieldConfig

* **displayName**: (type: string, default: `null`)
  
* **max**: (type: integer, default: `null`)
  
* **min**: (type: integer, default: `null`)
  
* **thresholdMode**: (type: string, default: `"absolute"`)
  
* **noValue**: (type: string, default: `null`)
  
* **unit**: (type: string, default: `"short"`)
  
* **width**: (type: integer, default: `null`)
  
#### #setGridPos

* **h**: (type: integer, default: `8`)
  Panel height.
* **w**: (type: integer, default: `12`)
  Panel width.
* **x**: (type: integer, default: `null`)
  Panel x position.
* **y**: (type: integer, default: `null`)
  Panel y position.
#### #setOptions

* **showHeader**: (type: boolean, default: `true`)
  

#### #addDataLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addPanelLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addMapping

* **from**: (type: string, default: `null`)
  
* **id**: (type: integer, default: `null`)
  
* **operator**: (type: string, default: `null`)
  
* **text**: (type: string, default: `null`)
  
* **to**: (type: string, default: `null`)
  
* **type**: (type: integer, default: `null`)
  
* **value**: (type: string, default: `null`)
  
#### #addOverride

* **matcher**: (type: oject, default: `null`)
  
* **properties**: (type: array, default: `null`)
  
#### #addThresholdStep

* **color**: (type: string, default: `null`)
  
* **value**: (type: integer, default: `null`)
  
#### #addTarget

* **target**: (type: object)
  


### panel.text.new



* **content**: (type: string, default: `null`)
  
* **datasource**: (type: string, default: `"default"`)
  
* **description**: (type: string, default: `null`)
  
* **mode**: (type: string, default: `"markdown"`)
  
* **repeat**: (type: string, default: `null`)
  
* **repeatDirection**: (type: string, default: `null`)
  
* **title**: (type: string, default: `null`)
  
* **transparent**: (type: boolean, default: `false`)
  

#### #setGridPos

* **h**: (type: integer, default: `8`)
  Panel height.
* **w**: (type: integer, default: `12`)
  Panel width.
* **x**: (type: integer, default: `null`)
  Panel x position.
* **y**: (type: integer, default: `null`)
  Panel y position.

#### #addPanelLink

* **targetBlank**: (type: boolean, default: `true`)
  
* **title**: (type: string, default: `null`)
  
* **url**: (type: string, default: `null`)
  
#### #addTarget

* **target**: (type: object)
  



## target



### target.prometheus.new



* **datasource**: (type: string, default: `"default"`)
  
* **expr**: (type: string, default: `null`)
  
* **format**: (type: string, default: `"time_series"`)
  
* **instant**: (type: boolean, default: `null`)
  
* **interval**: (type: string, default: `null`)
  
* **intervalFactor**: (type: integer, default: `null`)
  
* **legendFormat**: (type: string, default: `null`)
  





## template



### template.custom.new



* **allValue**: (type: string, default: `null`)
  
* **hide**: (type: integer, default: `0`)
  
* **includeAll**: (type: boolean, default: `false`)
  
* **label**: (type: string, default: `null`)
  
* **multi**: (type: boolean, default: `false`)
  
* **name**: (type: string, default: `null`)
  
* **query**: (type: string, default: `null`)
  
* **queryValue**: (type: string, default: `""`)
  
* **skipUrlSync**: (type: string, default: `false`)
  

#### #setCurrent

* **selected**: (type: boolean, default: `false`)
  
* **text**: (type: string, default: `null`)
  
* **value**: (type: string, default: `null`)
  



### template.datasource.new



* **hide**: (type: integer, default: `0`)
  
* **includeAll**: (type: boolean, default: `false`)
  
* **label**: (type: string, default: `null`)
  
* **multi**: (type: boolean, default: `false`)
  
* **name**: (type: string, default: `null`)
  
* **query**: (type: string, default: `null`)
  
* **refresh**: (type: integer, default: `1`)
  
* **regex**: (type: string, default: `null`)
  
* **skipUrlSync**: (type: string, default: `false`)
  

#### #setCurrent

* **selected**: (type: boolean, default: `false`)
  
* **text**: (type: string, default: `null`)
  
* **value**: (type: string, default: `null`)
  



### template.query.new



* **allValue**: (type: string, default: `null`)
  
* **datasource**: (type: string, default: `null`)
  
* **definition**: (type: string, default: `null`)
  
* **hide**: (type: integer, default: `0`)
  
* **includeAll**: (type: boolean, default: `false`)
  
* **label**: (type: string, default: `null`)
  
* **multi**: (type: boolean, default: `false`)
  
* **name**: (type: string, default: `null`)
  
* **query**: (type: string, default: `null`)
  
* **refresh**: (type: integer, default: `0`)
  
* **regex**: (type: string, default: `null`)
  
* **skipUrlSync**: (type: string, default: `false`)
  
* **sort**: (type: integer, default: `0`)
  
* **tagValuesQuery**: (type: string, default: `null`)
  
* **tags**: (type: array, default: `null`)
  
* **tagsQuery**: (type: string, default: `null`)
  
* **useTags**: (type: boolean, default: `false`)
  

#### #setCurrent

* **selected**: (type: boolean, default: `null`)
  
* **text**: (type: string, default: `null`)
  
* **value**: (type: string, default: `null`)
  

#### #addOption

* **selected**: (type: boolean, default: `true`)
  
* **text**: (type: string, default: `null`)
  
* **value**: (type: string, default: `null`)
  


