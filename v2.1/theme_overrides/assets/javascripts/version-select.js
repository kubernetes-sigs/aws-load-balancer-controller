document.addEventListener('DOMContentLoaded', () => {
    const verSelDom = document.querySelector('#version-select')
    const verSelListDom = verSelDom.querySelector('.mdc-list')

    const baseURL = verSelDom.dataset.baseUrl
    const absBaseURL = new URL(baseURL, window.location.href)
    var curDocVer = absBaseURL.pathname.match(/([^\/]*)\/?$/)[1]
    curDocVer = curDocVer || "latest"

    function updateVersionSelectDOM(verItems) {
        verItems.forEach((verItem) => {
            var rippleSpan = document.createElement('span')
            rippleSpan.classList.add("mdc-list-item__ripple")
            var textSpan = document.createElement('span')
            textSpan.classList.add("mdc-list-item__text")
            textSpan.innerHTML = verItem.title

            var verSelListItem = document.createElement('li');
            verSelListItem.setAttribute("role", "option")
            verSelListItem.setAttribute("data-value", verItem.version)
            verSelListItem.classList.add("mdc-list-item")
            if (verItem.version == curDocVer || verItem.aliases.includes(curDocVer)) {
                verSelListItem.classList.add("mdc-list-item--selected")
            }
            verSelListItem.appendChild(rippleSpan)
            verSelListItem.appendChild(textSpan)
            verSelListDom.appendChild(verSelListItem)
        })
    }

    function setupVersionSelectEventHandlers() {
        const verSelMDC = new mdc.select.MDCSelect(verSelDom);
        verSelMDC.listen('MDCSelect:change', () => {
            const chosenDocVer = verSelMDC.value
            const chosenDocURL = new URL("../" + chosenDocVer, absBaseURL)
            window.location.href = chosenDocURL
        });
    }

    const verJSONURL = new URL(baseURL + "/../versions.json", window.location.href)
    fetch(verJSONURL)
        .then(response => response.json())
        .then(updateVersionSelectDOM)
        .then(setupVersionSelectEventHandlers)
});