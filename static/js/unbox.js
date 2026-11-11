var ws
var data

var heartbeatTimer

var itemgetSound = new Audio("/static/mp3/itemget.mp3")

var rollerContainer = document.getElementsByClassName("roller-container")[0]
var roller = document.getElementsByClassName("roller")[0]
var infoWrapper = document.getElementsByClassName("info-wrapper")[0]
var infoImage = document.getElementsByClassName("info-image")[0]

var itemUserSpan = document.getElementsByClassName("item-user")[0]
var itemNameSpan = document.getElementsByClassName("item-name")[0]
var itemDescSpan = document.getElementsByClassName("item-desc")[0]

const FPS = 60
const DURATION = 12

var frame = 0
var totalDuration = DURATION
var totalFrame = totalDuration * FPS
var increment = 1 / totalFrame
var currentTime  = 0
var lastDistance = 0.0
var distance = 0.0
var maxWidth = 19700

// load audio context
window.AudioContext = window.AudioContext || window.webkitAudioContext
var tickaudioctx = new window.AudioContext()
var tickaudiobuf

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms))
}

function loadSound(url) {
    request = new XMLHttpRequest()
    request.open("GET", url, true)
    request.responseType = "arraybuffer"
    request.onload = () => {
        tickaudioctx.decodeAudioData(request.response, buffer => {
            if (!buffer) return
            tickaudiobuf = buffer
        })
    }
    request.onerror = () => {
        console.error("BufferLoader: XHR error")
    }
    request.send()
}

function playSound(buffer) {
    src = tickaudioctx.createBufferSource()
    src.buffer = buffer
    src.connect(tickaudioctx.destination)

    gain = tickaudioctx.createGain()
    gain.connect(tickaudioctx.destination)
    gain.gain.value = 1

    src.start(0) // start immediately
}

loadSound("/static/mp3/ticksound.mp3") // load the dang audio

function connect() {
    ws = new WebSocket(`wss://${window.location.host}/ws`)

    // send broadcaster id
    ws.onopen = _ => {
        id = window.location.pathname.split("/")[2]
        ws.send(JSON.stringify(id))

        refreshHeartbeat()
    }

    ws.onmessage = e => {
        data = JSON.parse(e.data)
        console.log(data)

        if (data == "ping") {
            refreshHeartbeat()
        } else {
            main()
        }
    }

    ws.onclose = e => {
        clearTimeout(heartbeatTimer)
        // setTimeout(connect, 5000)
    }

    ws.onerror = ws.close
}

function refreshHeartbeat() {
    clearTimeout(heartbeatTimer)

    heartbeatTimer = setTimeout(() => {
        console.warn("connecting closed because of missed heartbeat")
        ws.close()
    }, 6000)
}

function easeOutCubic(x) {
    return 1 - Math.pow(1 - x, 3)
}

function animate() {
    if (currentTime * totalDuration >= totalDuration) {
        cancelAnimationFrame(frame)
    } else {
        cv = easeOutCubic(currentTime)
        currentTime += increment
        rollerContainer.style.marginLeft = `-${cv * maxWidth}px`

        distance = cv * maxWidth

        if (distance >= (lastDistance + 256)) {
            lastDistance = distance
            playSound(tickaudiobuf)
        } else if (distance == maxWidth) {
            cancelAnimationFrame(frame)
            return
        }
    }

    frame = requestAnimationFrame(animate)
}

async function main() {
    for (let i = 0; i < data.item_sequence.length; i++) {
        const item = data.item_sequence[i];

        div = document.createElement("div")
        div.style.backgroundImage = `url("/${item.image_url}")`

        ans = ""
        if (item.rarity == "common") {
            ans = "188,230,255"
        } else if (item.rarity == "uncommon") {
            ans = "107,191,0"
        } else if (item.rarity == "rare") {
            ans = "0,181,242"
        } else if (item.rarity == "epic") {
            ans = "135,51,198"
        } else {
            ans = "247,205,15"
        }

        div.style.borderBottom = "20px solid rgba(" + ans + ",0.5)"

        // picked item
        if (i == 78) {
            itemUserSpan.innerText = data.user_login
            itemNameSpan.innerText = item.name
            itemNameSpan.style.color = "rgb(" + ans + ")"
            itemDescSpan.innerText = item.description
            infoImage.style.backgroundImage = div.style.backgroundImage
            infoImage.style.filter = "drop-shadow(rgba(" + ans + ",0.75) 0 0 2em)"
        }

        rollerContainer.append(div)
    }

    // show roller and dim background
    roller.classList.add("fade-visible")
    roller.classList.remove("fade-hidden")
    // document.body.classList.add("dimmed")

    await sleep(100)

    animate() // spin roller

    await sleep(6000)

    // hide roller
    roller.classList.add("fade-hidden")
    roller.classList.remove("fade-visible")

    // show item information
    infoWrapper.classList.add("fade-visible")
    infoWrapper.classList.remove("fade-hidden")

    // show item image and play sound
    infoImage.classList.add("spin-visible")
    infoImage.classList.remove("spin-hidden")
    itemgetSound.cloneNode().play() // PLAY_SOUND

    await sleep(2000)

    // hide item information and image
    infoImage.classList.add("spin-hidden")
    infoImage.classList.remove("spin-visible")
    infoWrapper.classList.add("fade-hidden")
    infoWrapper.classList.remove("fade-visible")

    // reset animation values
    frame = 0
    totalDuration = DURATION
    totalFrame = totalDuration * FPS
    increment = 1 / totalFrame
    currentTime  = 0
    lastDistance = 0.0
    distance = 0.0
    maxWidth = 19700

    // reset roller position
    rollerContainer.style.marginLeft = ""
    rollerContainer.innerHTML = ""

    // reset
    ws.send(JSON.stringify("done"))
}

document.addEventListener("DOMContentLoaded", connect)
window.addEventListener("beforeunload", (e) => {
    ws.close()
})
