(function ($) {
  const options = {}; // no options

  function init(plot) {
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

    function getFormatLabel(date) {
      // Format labels in accordance with xaxis tick size
      const { xaxis } = plot.getAxes();

      const d = new Date(date);
      if (xaxis.tickSize[1] === 'second') {
        return `${d.getHours()}:${d.getMinutes()}:${d.getSeconds()}`;
      }
      if (xaxis.tickSize[1] === 'minute') {
        return `${d.getHours()}:${d.getMinutes()}`;
      }
      if (xaxis.tickSize[1] === 'hour') {
        return `${d.getHours()}:${d.getMinutes()}`;
      }
      return `${d.getHours()}`;
    }

    const onHover = (target, position) => {
      this.tooltipY = target.currentTarget.getBoundingClientRect().bottom - 28;

      if (!this.selecting) {
        this.selectingFrom = {
          label: getFormatLabel(position.x),
          x: position.x,
          pageX: position.pageX,
        };
      } else {
        this.selectingTo = {
          label: getFormatLabel(position.x),
          x: position.x,
          pageX: position.pageX,
        };
      }
      updateTooltips();
    };

    const updateTooltips = () => {
      if (!this.selecting) {
        // If we arn't in selection mode
        this.$tooltip.html(this.selectingFrom.label).show();
        this.selectingFrom.width = $(this.$tooltip).outerWidth();
        setTooltipPosition(this.$tooltip, {
          x: this.selectingFrom.pageX,
          y: this.tooltipY,
        });
      } else {
        const { left } = $(this.$tooltip).offset();
        if (
          (this.selectingTo.pageX > left &&
            this.selectingTo.pageX < left + this.selectingFrom.width) ||
          (this.selectingTo.pageX < left &&
            this.selectingTo.pageX + this.selectingFrom.width > left)
        ) {
          // Intersection
          this.$tooltip2.hide();
          this.$tooltip.html(
            `${getFormatLabel(this.selectingFrom.x)} - ${getFormatLabel(
              this.selectingTo.x
            )}`
          );
          setTooltipPosition(this.$tooltip, {
            x: this.selectingFrom.pageX,
            y: this.tooltipY,
          });
        } else {
          // No intersection. Display two tooltips
          this.$tooltip.html(getFormatLabel(this.selectingFrom.x)).show();
          this.$tooltip2.html(getFormatLabel(this.selectingTo.x)).show();
          setTooltipPosition(this.$tooltip, {
            x: this.selectingFrom.pageX,
            y: this.tooltipY,
          });
          setTooltipPosition(this.$tooltip2, {
            x: this.selectingTo.pageX,
            y: this.tooltipY,
          });
        }
      }
    };

    const onLeave = () => {
      // Save tooltips while selecting
      if (!this.selecting) {
        this.$tooltip.hide();
        this.$tooltip2.hide();
      }
    };

    function onMove() {}

    const setTooltipPosition = ($tip, pos) => {
      const totalTipWidth = $tip.outerWidth();
      const totalTipHeight = $tip.outerHeight();
      if (
        pos.x - $(window).scrollLeft() >
        $(window).innerWidth() - totalTipWidth
      ) {
        pos.x -= totalTipWidth;
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
        left: `${pos.x}px`,
        top: `${pos.y}px`,
        right: 'auto',
      });
    };

    const onSelecting = () => {
      // Save selection state
      this.selecting = true;
    };

    const onSelected = () => {
      // Clean up selection state and hide tooltips
      this.selecting = false;
      this.$tooltip.hide();
      this.$tooltip2.hide();
    };

    const createDomElement = () => {
      if (this.$tooltip) return;
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
      const $tip = $('<div></div>');
      const $tip2 = $('<div></div>');

      $tip.appendTo('body').hide();
      $tip2.appendTo('body').hide();
      $tip.css({ position: 'absolute', left: 0, top: 0 });
      $tip.css(tooltipStyle);
      $tip2.css({ position: 'absolute', left: 0, top: 0 });
      $tip2.css(tooltipStyle);
      this.$tooltip = $tip;
      this.$tooltip2 = $tip2;
    };

    const destroyDomElements = () => {
      this.$tooltip.remove();
      this.$tooltip2.remove();
    };

    function bindEvents(plot, eventHolder) {
      plot.getPlaceholder().bind('plothover', onHover);
      plot.getPlaceholder().bind('plotselecting', onSelecting);
      plot.getPlaceholder().bind('plotselected', onSelected);

      $(eventHolder).bind('mousemove', onMove);
      $(eventHolder).bind('mouseout', onLeave);
    }

    function shutdown(plot, eventHolder) {
      plot.getPlaceholder().unbind('plothover', onHover);
      plot.getPlaceholder().unbind('plotselecting', onSelecting);
      plot.getPlaceholder().unbind('plotselected', onSelected);
      $(eventHolder).unbind('mousemove', onMove);
      $(eventHolder).unbind('mouseout', onLeave);
    }

    createDomElement();

    plot.hooks.bindEvents.push(bindEvents);
    plot.hooks.shutdown.push(shutdown);
  }

  ($ as any).plot.plugins.push({
    init,
    options,
    name: 'pyro-tooltip',
    version: '0.1',
  });
})(jQuery);
