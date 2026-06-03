(function () {
  var REPO = "kubernetes-sigs/aws-load-balancer-controller";
  var LABEL = "announcement";
  var API_URL =
    "https://api.github.com/repos/" +
    REPO +
    "/issues?labels=" +
    LABEL +
    "&state=open&per_page=5&sort=created&direction=desc";

  function escapeHtml(text) {
    var div = document.createElement("div");
    div.appendChild(document.createTextNode(text));
    return div.innerHTML;
  }

  function renderAnnouncements(issues) {
    if (issues.length === 0) return;

    var banner = document.getElementById("gh-announcements");
    if (!banner) return;

    var issueLinks = issues
      .map(function (issue) {
        return (
          '<li class="gh-announcement-item">' +
          '<a href="' + issue.html_url + '" target="_blank" rel="noopener">' +
          escapeHtml(issue.title) +
          "</a>" +
          "</li>"
        );
      })
      .join("");

    banner.innerHTML =
      '<details class="gh-announcement-details">' +
      '<summary class="gh-announcement-summary">' +
      "⚠️ Known Issues — " +
      "We are aware of some issues and are actively working on fixes. " +
      "Click to view details and workarounds." +
      "</summary>" +
      '<div class="gh-announcement-body">' +
      '<ul class="gh-announcement-list">' +
      issueLinks +
      "</ul>" +
      "</div>" +
      "</details>";

    banner.style.display = "block";
  }

  function fetchAnnouncements() {
    fetch(API_URL)
      .then(function (res) {
        if (!res.ok) throw new Error("GitHub API error: " + res.status);
        return res.json();
      })
      .then(renderAnnouncements)
      .catch(function (err) {
        console.warn("Could not load announcements:", err);
      });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", fetchAnnouncements);
  } else {
    fetchAnnouncements();
  }
})();
