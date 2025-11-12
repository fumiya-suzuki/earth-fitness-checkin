let currentUserId = null; 
const MAX_FALLBACK = 10; // Go側と合わせる

const checkinBtn = document.getElementById("checkinBtn"); 
const checkoutBtn = document.getElementById("checkoutBtn");

function showCheckinButton() { 
  checkinBtn.style.display = "inline-block"; 
  checkoutBtn.style.display = "none"; 
}

function showCheckoutButton() { 
  checkinBtn.style.display = "none"; 
  checkoutBtn.style.display = "inline-block"; 
}

async function init() { 
  await liff.init({ liffId: LIFF_ID });

  if (!liff.isLoggedIn()) { 
    liff.login(); 
    return; 
  }

  const profile = await liff.getProfile(); 
  currentUserId = profile.userId;

  try { 
    const res = await fetch(`/status?userId=${encodeURIComponent(currentUserId)}`);
    const data = await res.json();

    updateCapacityBar(data.count, MAX_FALLBACK);

    if (data.checkedIn) { 
      showCheckoutButton(); // ← チェックイン済みならチェックアウトだけ 
    } else {
      showCheckinButton(); 
    }
  } catch (e) { 
    console.error(e); 
    updateCapacityBar(0, MAX_FALLBACK); 
    console.error(e); 
    // 失敗したらとりあえずチェックインを出す 
    showCheckinButton(); 
  }
}

function updateCapacityBar(count, max) { 
  const realMax = max; 
  const percent = Math.min(100, Math.round((count / realMax) * 100)); 
  const fill = document.getElementById("capacityFill"); 
  const text = document.getElementById("capacityText"); 
  const icon = document.getElementById("capacityIcon"); 
  const percentLabel = document.getElementById("capacityPercent");

  // 幅 
  fill.style.width = percent + "%";

  if (percent > 69) {
    fill.style.background = "linear-gradient(90deg, #f43f5e 0%, #e11d48 100%)";
    icon.src = "./hard.png";
  } else if (percent > 49) {
    fill.style.background = "linear-gradient(90deg, #fb923c 0%, #f97316 100%)";
    icon.src = "./normal.png";
  } else {
    fill.style.background = "linear-gradient(90deg, #22c55e 0%, #16a34a 100%)";
    icon.src = "./good.png";
  }

  percentLabel.textContent = percent + "%";
  text.textContent = `混雑度：${count} / ${realMax}（${percent}%）`;
}


async function callApi(path, label) {
  const resultEl = document.getElementById("result");
  const targetBtn = path === '/checkin' ? checkinBtn : checkoutBtn;

  targetBtn.disabled = true;

  try {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ userId: currentUserId })
    });
    const data = await res.json();

    resultEl.innerText = `✅ ${label} 完了！`;
    updateCapacityBar(data.count, MAX_FALLBACK);

    if (path === '/checkin') {
      showCheckoutButton();
    } else {
      showCheckinButton();
    }
  } catch (e) { 
    console.error(e); 
    resultEl.innerText = `⚠ ${label} に失敗しました`;
  } finally { 
    // 戻す 
    targetBtn.disabled = false; 
    targetBtn.classList.remove('btn-loading'); 
  } 
}

document.getElementById("checkinBtn").onclick = () => callApi('/checkin', 'チェックイン');
document.getElementById("checkoutBtn").onclick = () => callApi('/checkout', 'チェックアウト');

init();
