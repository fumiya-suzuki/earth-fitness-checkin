let currentUserId = null; 
let currentDisplayName = "";
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

// プロフィール取得・表示制御 ------------------------------

async function ensureProfile(userId) {
  currentUserId = userId;

  const res = await fetch(`/member/profile?userId=${encodeURIComponent(userId)}`);
  if (!res.ok) {
    console.error("profile get error", res.status);
    // 失敗したら一旦フォーム出しておく（最悪ここで登録してもらう）
    showProfileForm();
    return;
  }

  const data = await res.json();

  if (!data.exists) {
    // まだフルネーム未登録 → フォームを表示する
    showProfileForm();
  } else {
    // 既に登録済み → フォームは隠して、普通にチェックイン UI を有効化
    hideProfileForm();
    enableCheckinUI();
  }
}

function showProfileForm() {
  const form = document.getElementById("profileForm");
  const msg = document.getElementById("profileMessage");

  if (form) form.style.display = "block";
  if (msg) {
    msg.style.display = "block";
    msg.textContent = "初回利用のため、お名前と会員種別の登録をお願いします。";
  }
  // プロフィール登録が終わるまではチェックイン系は無効にしておく
  if (checkinBtn) checkinBtn.disabled = true;
  if (checkoutBtn) checkoutBtn.disabled = true;
}

function hideProfileForm() {
  const form = document.getElementById("profileForm");
  const msg = document.getElementById("profileMessage");
  if (form) form.style.display = "none";
  if (msg) msg.style.display = "none";
}

function enableCheckinUI() {
  if (checkinBtn) checkinBtn.disabled = false;
  if (checkoutBtn) checkoutBtn.disabled = false;
}

async function submitProfile() {
  const lastNameEl = document.getElementById("lastName");
  const firstNameEl = document.getElementById("firstName");
  const msg = document.getElementById("profileMessage");

  const lastName = lastNameEl.value.trim();
  const firstName = firstNameEl.value.trim();
  const memberType = document.querySelector('input[name="memberType"]:checked')?.value;

  if (!lastName || !firstName) {
    msg.style.display = "block";
    msg.textContent = "姓と名を両方入力してください。";
    return;
  }

  try {
    const res = await fetch("/member/profile", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        userId: currentUserId,
        lastName,
        firstName,
        memberType,      // "general" or "1day"
        displayName: currentDisplayName,
      }),
    });

    if (!res.ok) {
      msg.style.display = "block";
      msg.textContent = "登録に失敗しました。時間をおいて再度お試しください。";
      return;
    }

    // 成功したらフォームを閉じて、チェックインUIを有効化
    hideProfileForm();
    enableCheckinUI();

  } catch (e) {
    console.error(e);
    msg.style.display = "block";
    msg.textContent = "通信エラーが発生しました。";
  }
}

// -------------------------------------------------------
// LIFF 初期化 & チェックイン画面初期化
// -------------------------------------------------------

async function init() { 
  await liff.init({ liffId: LIFF_ID });

  if (!liff.isLoggedIn()) { 
    liff.login(); 
    return; 
  }

  const profile = await liff.getProfile(); 
  currentUserId = profile.userId;
  currentDisplayName = profile.displayName;

  // ① プロフィール（フルネーム＆会員種別）を確認
  await ensureProfile(currentUserId);

  // ② 現在の混雑状況＆自分がチェックイン中かどうかを取得
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
      body: JSON.stringify({ userId: currentUserId, displayName: currentDisplayName, })
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

// ボタンクリック登録 & 初期化 --------------------------

document.getElementById("checkinBtn").onclick = () => callApi('/checkin', 'チェックイン');
document.getElementById("checkoutBtn").onclick = () => callApi('/checkout', 'チェックアウト');

// プロフィール登録ボタン
const profileSubmitBtn = document.getElementById("profileSubmitBtn");
if (profileSubmitBtn) {
  profileSubmitBtn.addEventListener("click", submitProfile);
}

init();
