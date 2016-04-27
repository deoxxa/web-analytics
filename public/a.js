(function() {
  var userID = '##USER_ID##';

  var qs = [
    'url=' + escape(document.location.toString()),
    'referer=' + escape(document.referrer),
  ].join('&');

  var q = [];
  var report = function(a, v) {
    q.push([a, v]);
  }

  report('view');

  (function ping() {
    report('ping');
    setTimeout(ping, 1000 * 30);
  })();

  var ajaxMode = false;

  function fallback() {
    ajaxMode = true;

    report = function(a, v) {
      var xhr = new XMLHttpRequest();
      xhr.open('POST', '/a/ev?' + qs);
      xhr.send(JSON.stringify({
        action: a,
        vars: v || {},
      }));
    };

    q.forEach(function(a) { report(a[0], a[1]); });
  }

  var fallbackTimeout = setTimeout(fallback, 1000 * 5);

  try {
    var s = new WebSocket('wss://' + document.location.host + '/a/ws?' + qs);

    s.addEventListener('open', function() {
      if (ajaxMode) {
        return;
      }

      s.addEventListener('close', fallback);
      s.addEventListener('error', fallback);

      clearTimeout(fallbackTimeout);

      report = function(a, v) {
        s.send(JSON.stringify({
          action: a,
          vars: v || {},
        }));
      };

      q.forEach(function(a) { report(a[0], a[1]); });
    });
  } catch (e) {
    clearTimeout(fallbackTimeout);

    fallback();
  }
})();
