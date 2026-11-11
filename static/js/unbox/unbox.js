import { SPINNER_CONFIG, rarityColors } from './config.js';

const unboxingWrapper = document.getElementById('unboxingWrapper');
const spinnerTape = document.getElementById('spinnerTape');
const spinnerTicker = document.getElementById('spinnerTicker');
const spinBtn = document.getElementById('spinBtn');
const revealModal = document.getElementById('revealModal');

const itemGetSound = new Audio("/static/mp3/item_get.mp3")
const tickSound = new Audio("/static/mp3/tick.mp3")

let generatedItems = [];
let animationFrameId = 0;
let isSpinning = false;
let autoCloseTimeoutId = null;

let itemPool = []
let userLogin = ""
let winningItemJson = ""

// websocket
var heartbeatTimer

import { config } from "./config.js";

function refreshHeartbeat() {
  clearTimeout(heartbeatTimer)

  heartbeatTimer = setTimeout(() => {
      console.warn("connecting closed because of missed heartbeat")
      ws.close()
  }, 6000)
}

const ws = new WebSocket(config.wsUrl)
ws.addEventListener("open", ev => {
  console.log(`ws open: ${ev}`)
  let id = window.location.pathname.split("/")[2]
  ws.send(JSON.stringify(id))
})
ws.addEventListener("close", ev => {
  console.log(`ws close: ${ev}`)
})
ws.addEventListener("message", ev => {
  let data = JSON.parse(ev.data)

  if (data == "ping") {
    console.log("ws message: ping")
    refreshHeartbeat()
  } else {
    console.log("ws message: not ping")
    itemPool = data.items
    userLogin = data.user_login
    triggerUnboxing()
  }
})
ws.addEventListener("error", ev => {
  console.error(`ws error: ${ev}`)
})

function getRandomItemByRarity() {
  const roll = Math.random() * 100;
  let currentWeightSum = 0;
  let chosenRarity = 'common';

  for (const [rarity, weight] of Object.entries(SPINNER_CONFIG.rarityWeights)) {
    currentWeightSum += weight;
    if (roll <= currentWeightSum) {
      chosenRarity = rarity;
      break;
    }
  }

  const matchingItems = itemPool.filter(item => item.rarity === chosenRarity);
  if (matchingItems.length === 0) {
    return itemPool[Math.floor(Math.random() * itemPool.length)];
  }
  return matchingItems[Math.floor(Math.random() * matchingItems.length)];
}

function populateTape() {
  spinnerTape.innerHTML = '';
  generatedItems = [];
  spinnerTape.style.transform = 'translateX(0px)';
  spinnerTicker.style.transform = 'rotate(0deg)';

  let forcedItemObj = null;
  if (SPINNER_CONFIG.forcedWinner) {
    forcedItemObj = itemPool.find(item => item.name.toLowerCase() === SPINNER_CONFIG.forcedWinner.toLowerCase());
  }

  for (let i = 0; i < SPINNER_CONFIG.totalTapeCards; i++) {
    let selectedItem;
    if (i === SPINNER_CONFIG.winningIndex && forcedItemObj) {
      selectedItem = forcedItemObj;
    } else {
      selectedItem = getRandomItemByRarity();
    }

    generatedItems.push(selectedItem);

    const card = document.createElement('div');
    card.className = `item-card ${selectedItem.rarity}`;

    // UPDATED HTML TEMPLATE: Swapped old .item-icon container for a crisp image tag
    let imageUrl = `${config.imgUrl}/${selectedItem.image_url}`
    card.innerHTML = `
      <img class="item-card-image" src="${imageUrl}" alt="${selectedItem.name}">
      <div class="item-name">${selectedItem.name}</div>
    `;
    spinnerTape.appendChild(card);
  }

  document.documentElement.style.setProperty('--dynamic-glow', 'rgba(156, 39, 176, 0.2)');
  document.documentElement.style.setProperty('--dynamic-glow-solid', 'rgba(156, 39, 176, 0.5)');
}

function startUnboxing() {
  if (isSpinning) return;
  isSpinning = true;
  if (spinBtn) spinBtn.disabled = true;
  populateTape();

  // 🎬 TRIGGER DIMMING: Tells style.css to smoothly darken everything behind the track
  document.body.classList.add('cinematic-dim');

  unboxingWrapper.classList.add('show-structure');
  void unboxingWrapper.offsetHeight;

  const targetItemOffset = SPINNER_CONFIG.winningIndex * SPINNER_CONFIG.cardWidth;
  const maxWiggleRange = SPINNER_CONFIG.cardWidth - 40;
  const innerCardVariation = Math.floor(Math.random() * maxWiggleRange) + 20 - (SPINNER_CONFIG.cardWidth / 2);

  const totalDistance = targetItemOffset + innerCardVariation;
  const totalAnimationDuration = SPINNER_CONFIG.cruiseDuration + SPINNER_CONFIG.slowdownDuration;

  const slopeMultiplier = SPINNER_CONFIG.startupAggression;
  const velocityRatio = (slopeMultiplier * SPINNER_CONFIG.cruiseDuration) / SPINNER_CONFIG.slowdownDuration;

  const decelerateDistance = totalDistance / (1 + velocityRatio);
  const cruiseDistance = totalDistance - decelerateDistance;
  const topSpeed = cruiseDistance / SPINNER_CONFIG.cruiseDuration;

  let startTime = null;
  let lastCardPassed = -1;
  let tickerResetTimeout = null;

  function animate(timestamp) {
    if (!startTime) startTime = timestamp;
    const elapsed = timestamp - startTime;

    if (elapsed >= 150) {
      unboxingWrapper.classList.add('fade-in');
    }

    let currentX = 0;
    let currentVelocity = 0;

    if (elapsed < SPINNER_CONFIG.cruiseDuration) {
      currentX = topSpeed * elapsed;
      currentVelocity = topSpeed;
    } else if (elapsed < totalAnimationDuration) {
      const t = (elapsed - SPINNER_CONFIG.cruiseDuration) / SPINNER_CONFIG.slowdownDuration;
      const easeOutCustom = 1 - Math.pow(1 - t, SPINNER_CONFIG.startupAggression);
      currentX = cruiseDistance + (decelerateDistance * easeOutCustom);

      currentVelocity = (decelerateDistance / SPINNER_CONFIG.slowdownDuration) *
                        SPINNER_CONFIG.startupAggression * Math.pow(1 - t, SPINNER_CONFIG.startupAggression - 1);
    } else {
      currentX = totalDistance;
      spinnerTape.style.transform = `translateX(${-currentX}px)`;
      spinnerTicker.style.transform = 'rotate(0deg)';

      const finalWinnerItem = generatedItems[SPINNER_CONFIG.winningIndex];
      showPrize(finalWinnerItem);
      return;
    }

    spinnerTape.style.transform = `translateX(${-currentX}px)`;

    const baselineShift = (SPINNER_CONFIG.cardWidth / 2) - innerCardVariation;
    const currentCardIndex = Math.floor((currentX + baselineShift) / SPINNER_CONFIG.cardWidth);

    if (currentCardIndex !== lastCardPassed && currentCardIndex >= 0 && currentCardIndex < generatedItems.length) {
      const relativeSpeedModifier = currentVelocity / topSpeed;
      tickSound.cloneNode().play()
      lastCardPassed = currentCardIndex;

      const activeCardItem = generatedItems[currentCardIndex];
      const colorScheme = rarityColors[activeCardItem.rarity];
      if (colorScheme) {
        document.documentElement.style.setProperty('--dynamic-glow', colorScheme.alpha);
        document.documentElement.style.setProperty('--dynamic-glow-solid', colorScheme.solid);
      }

      const bendAngle = Math.min(currentVelocity * SPINNER_CONFIG.tickerWiggleIntensity, SPINNER_CONFIG.maxWiggleAngle);
      spinnerTicker.style.transform = `rotate(${-bendAngle}deg)`;

      if (tickerResetTimeout) clearTimeout(tickerResetTimeout);
      const adaptiveRecoveryTime = Math.max(20, 45 * relativeSpeedModifier);

      tickerResetTimeout = setTimeout(() => {
        if (isSpinning) {
          spinnerTicker.style.transform = 'rotate(0deg)';
        }
      }, adaptiveRecoveryTime);
    }

    animationFrameId = requestAnimationFrame(animate);
  }

  animationFrameId = requestAnimationFrame(animate);
}

function showPrize(item) {
  // Smoothly hide the horizontal spinner track view as the reveal triggers
  unboxingWrapper.classList.add('hide-spinner');

  // Update frameless field contents
  document.getElementById('revealRarity').innerText = item.rarity;
  document.getElementById('revealRarity').style.color = `var(--${item.rarity})`;

  let imageUrl = `${window.location.protocol}//${window.location.host}/${item.image_url}`
  document.getElementById('revealIcon').innerHTML = `<img class="reveal-prize-image" src="${imageUrl}" alt="${item.name}">`;

  // Inject the customizable description text string smoothly
  document.getElementById('revealName').innerText = `${userLogin} unboxed ${item.name}!`;
  document.getElementById('revealDescription').innerText = item.description || "";

  // Dynamic Aura: Sets the large circular backer glow color variables to match item rarity
  const finalColors = rarityColors[item.rarity];
  if (finalColors) {
    document.documentElement.style.setProperty('--dynamic-glow', finalColors.alpha);
    document.documentElement.style.setProperty('--dynamic-glow-solid', finalColors.solid);
    document.documentElement.style.setProperty('--active-winner-color', finalColors.solid);
  }

  itemGetSound.play()

  revealModal.classList.add('active');

  // TWITCH: send the unboxed item back to ebs
  winningItemJson = JSON.stringify(item)

  // Triggers the auto Close delay sequence configured in config.js
  if (autoCloseTimeoutId) clearTimeout(autoCloseTimeoutId);
  autoCloseTimeoutId = setTimeout(() => {
    closeModal();
  }, SPINNER_CONFIG.autoCloseDelay);
}

function closeModal() {
  if (autoCloseTimeoutId) clearTimeout(autoCloseTimeoutId);

  revealModal.classList.remove('active');
  unboxingWrapper.classList.remove('fade-in');

  // 🚪 RESTORE LIGHTING: Clears out dark overlay tint alongside structural collapse timings
  document.body.classList.remove('cinematic-dim');

  setTimeout(() => {
    unboxingWrapper.classList.remove('show-structure');
    unboxingWrapper.classList.remove('hide-spinner');
    if (spinBtn) spinBtn.disabled = false;
    isSpinning = false;

    console.log(`ws send ${winningItemJson}`)
    ws.send(winningItemJson)
  }, 410);
}

function triggerUnboxing(forcedItemName=null) {
  if (isSpinning) return;
  if (forcedItemName) {
    SPINNER_CONFIG.forcedWinner = forcedItemName;
  }

  if (autoCloseTimeoutId) clearTimeout(autoCloseTimeoutId);

  unboxingWrapper.classList.remove('fade-in');
  unboxingWrapper.classList.remove('show-structure');
  unboxingWrapper.classList.remove('hide-spinner'); // Ensure layout state is clean
  document.body.classList.remove('cinematic-dim'); // Wipe dim states on remote re-triggers
  revealModal.classList.remove('active');

  setTimeout(() => {
    startUnboxing();
  }, 50);
};

// Hook close buttons manually to prevent layout reference breaks
const closeBtn = document.querySelector('.btn-close');
if (closeBtn) closeBtn.addEventListener('click', closeModal);

// Programmatically hook the "Open Case" button if it is rendered visible
if (spinBtn) {
  spinBtn.addEventListener('click', startUnboxing);
}

// Programmatically hook the "Dismiss" modal close button
const closeModalBtn = document.getElementById('closeModalBtn');
if (closeModalBtn) {
  closeModalBtn.addEventListener('click', closeModal);
}

// Optional fallback: lets you close the modal if clicking outside the reward card content frame
if (revealModal) {
  revealModal.addEventListener('click', (e) => {
    if (e.target === revealModal) closeModal();
  });
}
