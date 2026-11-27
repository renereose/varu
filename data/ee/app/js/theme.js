// Runs before the page draws, so it opens in the theme chosen on this device
// (light or dark; anything else follows the device). Same keys as pref() in
// common.js.
(function () {
  var theme = (document.cookie.match(/(?:^|; )varu_theme=([^;]*)/) || [])[1];
  if (theme === undefined) {
    try { theme = localStorage.getItem("varu.theme"); } catch (e) { /* private mode */ }
  }
  if (theme === "light" || theme === "dark") document.documentElement.dataset.theme = theme;
})();
