import { format } from 'date-fns';

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

      if (!xaxis) {
        return '';
      }

      try {
        const d = new Date(date);

        const hours = Math.abs(xaxis.max - xaxis.min) / 60 / 60 / 1000;

        if (hours < 12) {
          return format(d, 'HH:mm:ss');
        }
        if (hours > 12 && hours < 24) {
          return format(d, 'HH:mm');
        }
        if (hours > 24) {
          return format(d, 'MMM do HH:mm');
        }
        return format(d, 'MMM do HH:mm');
      } catch (e) {
        return '???';
      }
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
      } else if (
        (this.selectingTo.pageX - this.selectingFrom.width / 2 >
          this.selectingFrom.pageX - this.selectingFrom.width &&
          this.selectingTo.pageX - this.selectingFrom.width / 2 <
            this.selectingFrom.pageX + this.selectingFrom.width) ||
        (this.selectingFrom.pageX - this.selectingTo.pageX > 0 &&
          this.selectingFrom.pageX - this.selectingTo.pageX <
            this.selectingFrom.width) // Check if tooltips can intersect
      ) {
        // Render Intersection
        this.$tooltip2.hide();
        this.$tooltip.html(
          `${getFormatLabel(
            Math.min(this.selectingFrom.x, this.selectingTo.x)
          )} - 
             ${getFormatLabel(
               Math.max(this.selectingFrom.x, this.selectingTo.x)
             )}`
        );

        // Stick to left selection
        setTooltipPosition(
          this.$tooltip,
          {
            x: Math.min(this.selectingFrom.pageX, this.selectingTo.pageX),
            y: this.tooltipY,
          },
          false
        );
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
    };

    const onLeave = () => {
      // Save tooltips while selecting
      if (!this.selecting) {
        this.$tooltip.hide();
        this.$tooltip2.hide();
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

    const onSelecting = (evt) => {
      // Save selection state
      this.selecting = true;
    };

    const onSelected = () => {
      // Clean up selection state and hide tooltips
      this.selecting = false;
      this.$tooltip.hide();
      this.$tooltip2.hide();
    };

    const onMouseUp = () => {
      this.selecting = false;
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
      const $tip = $('<div data-testid="timeline-tooltip1"></div>');
      const $tip2 = $('<div data-testid="timeline-tooltip2"></div>');

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
      plot.getPlaceholder().bind('plotselected', onSelected);

      $(eventHolder).bind('mousemove', onMove);
      $(eventHolder).bind('mouseout', onLeave);
      $(eventHolder).bind('mouseup', onMouseUp);
      $(eventHolder).bind('mousedown', onSelecting);
    }

    function shutdown(plot, eventHolder) {
      plot.getPlaceholder().unbind('plothover', onHover);
      // plot.getPlaceholder().unbind('plotselecting', onSelecting);
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
