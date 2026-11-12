import { config } from "../config.js"

function oauth2popup() {
    let w = window.open(`${config.authUrl}/authorize`, "oauth2 login", "width=600,height=300")
    let t = setInterval(() => {
        if (w.closed) {
            clearInterval(t)
            window.location.reload()
        }
    }, 500)
}
async function request(method, route, token, data=null) {
    const url = `${config.apiUrl}/${route}`
    const response = await fetch(url, {
        method: method,
        mode: "cors",
        headers: {"Authorization": `Bearer ${token}`},
        body: data
    })

    let json = await response.json()
    return json
}

window.Twitch.ext.onAuthorized(auth => {
    console.log(auth)

    let authLink = document.querySelector("#auth-link")
    let content = document.querySelector("#content")

    request("GET", "broadcasters", auth.token)
    .then(json => {
        console.log(json)
        if (json.user_id == "") {
            authLink.style.display = "block"
        } else {
            content.style.display = "block"
        }
    })
    .catch(error => console.error)

})

window.Twitch.ext.onContext((context) => {
    document.body.classList.add(`${context.theme}-theme`)
});
