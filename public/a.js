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

  try {
    var s = new WebSocket('ws://127.0.0.1:5001/a/ws?' + qs);

    s.addEventListener('open', function() {
      report = function(a, v) {
        s.send(JSON.stringify({
          action: a,
          vars: v || {},
        }));
      };

      q.forEach(function(a) { report(a[0], a[1]); });
    });
  } catch (e) {
    report = function(a, v) {
      var xhr = new XMLHttpRequest();
      xhr.open('POST', '/a/ev?' + qs);
      xhr.send(JSON.stringify({
        action: a,
        vars: v || {},
      }));
    };

    q.forEach(function(a) { report(a[0], a[1]); });

    (function ping() {
      report('ping');
      setTimeout(ping, 1000 * 30);
    })();
  }
})();
