/* eslint-disable */
// @ts-nocheck
(function ($, e: any, t) {
  '$:nomunge';
  var i: any = [],
    n = ($.resize = $.extend($.resize, {})),
    a: any,
    r: any = false,
    s = 'setTimeout',
    u = 'resize',
    m = u + '-special-event',
    o = 'pendingDelay',
    l = 'activeDelay',
    f = 'throttleWindow';
  n[o] = 200;
  n[l] = 20;
  n[f] = true;
  $.event.special[u] = {
    setup: function () {
      if (!n[f] && this[s]) {
        return false;
      }
      var e = $(this);
      i.push(this);
      e.data(m, { w: e.width(), h: e.height() });
      if (i.length === 1) {
        a = t;
        h(undefined); // h() == h(undefined)
      }
    },
    teardown: function () {
      if (!n[f] && this[s]) {
        return false;
      }
      var e = $(this);
      for (var t = i.length - 1; t >= 0; t--) {
        if (i[t] == this) {
          i.splice(t, 1);
          break;
        }
      }
      e.removeData(m);
      if (!i.length) {
        if (r) {
          cancelAnimationFrame(a);
        } else {
          clearTimeout(a);
        }
        a = null;
      }
    },
    add: function (e: any) {
      if (!n[f] && this[s]) {
        return false;
      }
      var i: any;
      function a(e: any, n: any, a: any) {
        // @ts-ignore
        var r = $(this),
          s = r.data(m) || {};
        s.w = n !== t ? n : r.width();
        s.h = a !== t ? a : r.height();
        // @ts-ignore
        i.apply(this, arguments);
      }
      if ($.isFunction(e)) {
        i = e;
        return a;
      } else {
        i = e.handler;
        e.handler = a;
      }
    },
  };
  function h(t: any) {
    if (r === true) {
      r = t || 1;
    }
    for (var s = i.length - 1; s >= 0; s--) {
      var l = $(i[s]);
      if (l[0] == e || l.is(':visible')) {
        var f = l.width(),
          c = l.height(),
          d = l.data(m);
        if (d && (f !== d.w || c !== d.h)) {
          l.trigger(u, [(d.w = f), (d.h = c)]);
          r = t || true;
        }
      } else {
        d = l.data(m);
        d.w = 0;
        d.h = 0;
      }
    }
    if (a !== null) {
      if (r && (t == null || t - r < 1e3)) {
        a = e.requestAnimationFrame(h);
      } else {
        a = setTimeout(h, n[o]);
        r = false;
      }
    }
  }
  if (!e.requestAnimationFrame) {
    e.requestAnimationFrame = (function () {
      return (
        e.webkitRequestAnimationFrame ||
        e.mozRequestAnimationFrame ||
        e.oRequestAnimationFrame ||
        e.msRequestAnimationFrame ||
        function (t: any, i: any) {
          return e.setTimeout(function () {
            t(new Date().getTime());
          }, n[l]);
        }
      );
    })();
  }
  if (!e.cancelAnimationFrame) {
    e.cancelAnimationFrame = (function () {
      return (
        e.webkitCancelRequestAnimationFrame ||
        e.mozCancelRequestAnimationFrame ||
        e.oCancelRequestAnimationFrame ||
        e.msCancelRequestAnimationFrame ||
        clearTimeout
      );
    })();
  }
  // @ts-ignore
})(jQuery, window);

(function ($) {
  var options = {}; // no options

  function init(plot: any) {
    function onResize() {
      var placeholder = plot.getPlaceholder();

      // somebody might have hidden us and we can't plot
      // when we don't have the dimensions
      if (placeholder.width() == 0 || placeholder.height() == 0) return;

      plot.resize();
      plot.setupGrid();
      plot.draw();
    }

    function bindEvents(plot: any, eventHolder: any) {
      plot.getPlaceholder().resize(onResize);
    }

    function shutdown(plot: any, eventHolder: any) {
      plot.getPlaceholder().unbind('resize', onResize);
    }

    plot.hooks.bindEvents.push(bindEvents);
    plot.hooks.shutdown.push(shutdown);
  }

  $.plot.plugins.push({
    init: init,
    options: options,
    name: 'resize',
    version: '1.0',
  });
  // @ts-ignore
})(jQuery);
