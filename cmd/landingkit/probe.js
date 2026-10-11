/* landingkit verify's background probe. Injected, after a line that sets
   window.__landingkitPoints = [[x, y], ...], into a COPY of the built page
   (probe.html beside index.html); the page itself is screenshotted unchanged.

   It keeps the candidate points where nothing but the page background is
   painted, and writes them, with the viewport height, on <html> for
   Chrome's --dump-dom to print. "Nothing painted" is decided over EVERY
   element, not only the ones elementsFromPoint returns: that list skips
   pointer-events:none, and a transparent element on top would hide a tinted
   one beneath it. An element paints at a point when the point is inside its
   border box and it has a background colour or image, or the point is on one
   of its borders; when the point is inside its box-shadow or outline; when it
   is replaced content (an image, an SVG, a form control); or when one of its
   ::before/::after boxes has a background (the parent's box stands in for
   the pseudo's, which a script cannot measure). html and body are the page
   background itself and are never excluded. */
(function () {
  function visible(c) { return c && c !== 'transparent' && !/^rgba\(.*,\s*0\)$/.test(c); }
  function px(v) { return parseFloat(v) || 0; }
  function paints(c) { return visible(c.backgroundColor) || (c.backgroundImage && c.backgroundImage !== 'none'); }
  function inside(r, x, y) { return x >= r.left && x < r.right && y >= r.top && y < r.bottom; }
  function grow(r, d) { return { left: r.left - d.l, right: r.right + d.r, top: r.top - d.t, bottom: r.bottom + d.b }; }
  // The box a list of shadows can reach: offset, then spread plus blur.
  function shadowReach(s) {
    var d = { l: 0, r: 0, t: 0, b: 0 };
    if (!s || s === 'none') return null;
    s.split(/,(?![^(]*\))/).forEach(function (one) {
      if (/\binset\b/.test(one)) return;
      var n = (one.replace(/rgba?\([^)]*\)/g, '').match(/-?[\d.]+px/g) || []).map(px);
      var ox = n[0] || 0, oy = n[1] || 0, reach = Math.max(0, (n[2] || 0) + (n[3] || 0));
      d.l = Math.max(d.l, reach - ox); d.r = Math.max(d.r, reach + ox);
      d.t = Math.max(d.t, reach - oy); d.b = Math.max(d.b, reach + oy);
    });
    return d;
  }
  var replaced = /^(IMG|CANVAS|VIDEO|IFRAME|INPUT|TEXTAREA|SELECT|BUTTON|OBJECT|EMBED)$/;

  function painters() {
    var out = [];
    document.querySelectorAll('body *').forEach(function (e) {
      var c = getComputedStyle(e), r = e.getBoundingClientRect();
      if (c.display === 'none' || c.visibility === 'hidden' || (r.width === 0 && r.height === 0)) return;
      if (paints(c) || replaced.test(e.tagName) || (e instanceof SVGElement)) out.push({ box: r });
      var bw = { t: px(c.borderTopWidth), r: px(c.borderRightWidth), b: px(c.borderBottomWidth), l: px(c.borderLeftWidth) };
      if (bw.t + bw.r + bw.b + bw.l > 0) out.push({ box: r, border: bw });
      var sh = shadowReach(c.boxShadow);
      if (sh) out.push({ box: grow(r, sh) });
      if (c.outlineStyle !== 'none' && px(c.outlineWidth) > 0) {
        var o = px(c.outlineWidth) + Math.abs(px(c.outlineOffset));
        out.push({ box: grow(r, { l: o, r: o, t: o, b: o }) });
      }
      ['::before', '::after'].forEach(function (p) {
        var pc = getComputedStyle(e, p);
        if (pc.content !== 'none' && pc.content !== 'normal' && (paints(pc) || shadowReach(pc.boxShadow))) out.push({ box: r });
      });
    });
    return out;
  }

  function onBorder(p, x, y) {
    var r = p.box, b = p.border;
    return (b.t > 0 && y < r.top + b.t + 1) || (b.b > 0 && y >= r.bottom - b.b - 1) ||
      (b.l > 0 && x < r.left + b.l + 1) || (b.r > 0 && x >= r.right - b.r - 1);
  }

  function run() {
    var ps = painters(), keep = [];
    (window.__landingkitPoints || []).forEach(function (pt) {
      var x = pt[0], y = pt[1];
      if (x >= innerWidth || y >= innerHeight) return;
      for (var i = 0; i < ps.length; i++) {
        if (inside(ps[i].box, x, y) && (!ps[i].border || onBorder(ps[i], x, y))) return;
      }
      keep.push(x + ',' + y);
    });
    var html = document.documentElement;
    html.setAttribute('data-landingkit-viewport', innerWidth + 'x' + innerHeight);
    html.setAttribute('data-landingkit-background', keep.join(' '));
  }
  // After load, so that images have their size, and one task later, so that
  // the page's own load handlers have run.
  addEventListener('load', function () { setTimeout(run, 0); });
})();
