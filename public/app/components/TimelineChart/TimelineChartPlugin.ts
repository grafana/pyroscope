// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-nocheck
import getFormatLabel from './getFormatLabel';

(function ($) {
  const options = {}; // no options

  function init(plot) {
    const plotOptions = plot.getOptions();

    this.selecting = false;
    this.tooltipY = 0;
    this.selectingFrom = {
      label: '',
      x: 0,
      pageX: 0,
      width: 0,
    };
    this.selectingTo = {
      label: '',
      x: 0,
      pageX: 0,
      width: 0,
    };

    const onPlotHover = (target, position) => {
      const { xaxis } = plot.getAxes();

      this.tooltipY = target.currentTarget.getBoundingClientRect().bottom - 28;
      if (!position.x) {
        return;
      }
      if (!this.selecting) {
        this.selectingFrom = {
          label: getFormatLabel({
            date: position.x,
            xaxis,
            timezone: plotOptions.xaxis.timezone,
          }),
          x: position.x,
          pageX: position.pageX,
        };
      } else {
        this.selectingTo = {
          label: getFormatLabel({
            date: position.x,
            xaxis,
            timezone: plotOptions.xaxis.timezone,
          }),
          x: position.x,
          pageX: position.pageX,
        };
      }
      updateTooltips();
    };

    const updateTooltips = () => {
      const { xaxis } = plot.getAxes();

      if (!this.selecting) {
        // If we arn't in selection mode
        this.$tooltip.html(this.selectingFrom.label).show();
        this.selectingFrom.width = $(this.$tooltip).outerWidth();
        setTooltipPosition(this.$tooltip, {
          x: this.selectingFrom.pageX,
          y: this.tooltipY,
        });
      } else {
        // Render Intersection
        this.$tooltip.html(
          `${getFormatLabel({
            date: Math.min(this.selectingFrom.x, this.selectingTo.x),
            xaxis,
            timezone: plotOptions.xaxis.timezone,
          })} - 
             ${getFormatLabel({
               date: Math.max(this.selectingFrom.x, this.selectingTo.x),
               xaxis,
               timezone: plotOptions.xaxis.timezone,
             })}`
        );

        // Stick to left selection
        setTooltipPosition(this.$tooltip, {
          x: this.selectingTo.pageX,
          y: this.tooltipY,
        });
      }
    };

    const onLeave = () => {
      // Save tooltips while selecting
      if (!this.selecting) {
        this.$tooltip.hide();
      }
    };

    function onMove() {}

    const setTooltipPosition = ($tip, pos, center = true) => {
      const totalTipWidth = $tip.outerWidth();
      const totalTipHeight = $tip.outerHeight();
      if (
        pos.x - $(window).scrollLeft() >
        $(window).innerWidth() - totalTipWidth
      ) {
        pos.x -= center ? totalTipWidth / 2 : totalTipWidth;
        pos.x = Math.max(pos.x, 0);
        $tip.css({
          left: 'auto',
          right: `0px`,
          top: `${pos.y}px`,
        });
        return;
      }
      if (
        pos.y - $(window).scrollTop() >
        $(window).innerWidth() - totalTipHeight
      ) {
        pos.y -= totalTipHeight;
      }

      $tip.css({
        left: `${pos.x - (center ? Math.floor(totalTipWidth / 2) : 0)}px`,
        top: `${pos.y}px`,
        right: 'auto',
      });
    };

    const onSelected = () => {
      // Clean up selection state and hide tooltips
      this.selecting = false;
      this.$tooltip.hide();
    };

    // Trying to mimic flot.selection.js
    const onMouseDown = () => {
      // Save selection state
      this.selecting = true;
    };

    const onMouseUp = () => {
      this.selecting = false;
    };

    const createDomElement = () => {
      if (this.$tooltip) {
        return;
      }
      const tooltipStyle = {
        background: '#fff',
        color: 'black',
        'z-index': '1040',
        padding: '0.4em 0.6em',
        'border-radius': '0.5em',
        'font-size': '0.8em',
        border: '1px solid #111',
        'white-space': 'nowrap',
      };
      const $tip = $('<div data-testid="timeline-tooltip1"></div>');

      $tip.appendTo('body').hide();
      $tip.css({ position: 'absolute', left: 0, top: 0 });
      $tip.css(tooltipStyle);
      this.$tooltip = $tip;
    };

    function bindEvents(plot, eventHolder) {
      const o = plot.getOptions();

      if (o.onHoverDisplayTooltip) {
        return;
      }

      plot.getPlaceholder().bind('plothover', onPlotHover);
      plot.getPlaceholder().bind('plotselected', onSelected);

      $(eventHolder).bind('mousemove', onMove);
      $(eventHolder).bind('mouseout', onLeave);
      $(eventHolder).bind('mouseleave', onLeave);

      $(eventHolder).bind('mouseup', onMouseUp);
      $(eventHolder).bind('mousedown', onMouseDown);
    }

    function shutdown(plot, eventHolder) {
      const o = plot.getOptions();

      if (o.onHoverDisplayTooltip) {
        return;
      }

      plot.getPlaceholder().unbind('plothover', onPlotHover);
      // plot.getPlaceholder().unbind('plotselecting', onSelecting);
      plot.getPlaceholder().unbind('plotselected', onSelected);
      $(eventHolder).unbind('mousemove', onMove);
      $(eventHolder).unbind('mouseout', onLeave);
      $(eventHolder).unbind('mouseleave', onLeave);
    }

    createDomElement();

    plot.hooks.bindEvents.push(bindEvents);
    plot.hooks.shutdown.push(shutdown);
  }

  $.plot.plugins.push({
    init,
    options,
    name: 'pyro-tooltip',
    version: '0.1',
  });
})(jQuery);
