let currentUserId = null;
let currentDisplayName = "";
const MAX_FALLBACK = 10; // Go側と合わせる
const host = window.location.hostname;
const USE_LIFF = host !== "localhost" && host !== "127.0.0.1" && host !== "::1";

const LOCAL_DEV_USER = {
  userId: "local-dev-user",
  displayName: "Local Dev",
};

const resultEl = document.getElementById("result");

function showResultMessage(message, isError = false) {
  if (!resultEl) {
    return;
  }
  resultEl.textContent = message;
  resultEl.classList.remove("text-success", "text-danger");
  resultEl.classList.add(isError ? "text-danger" : "text-success");
}

// プロフィール取得・表示制御 ------------------------------

async function ensureProfile(userId) {
  currentUserId = userId;

  const res = await fetch(`/member/profile?userId=${encodeURIComponent(userId)}`);
  if (!res.ok) {
    console.error("profile get error", res.status);
    showProfileForm();
    return false;
  }

  const data = await res.json();

  if (!data.exists) {
    showProfileForm();
    return false;
  }

  hideProfileForm();
  return true;
}

function showProfileForm() {
  const form = document.getElementById("profileForm");
  const msg = document.getElementById("profileMessage");

  if (form) form.style.display = "block";
  if (msg) {
    msg.style.display = "block";
    msg.textContent = "初回利用のため、お名前と会員種別の登録をお願いします。";
  }
}

function hideProfileForm() {
  const form = document.getElementById("profileForm");
  const msg = document.getElementById("profileMessage");
  if (form) form.style.display = "none";
  if (msg) msg.style.display = "none";
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
        memberType,
        displayName: currentDisplayName,
      }),
    });

    if (!res.ok) {
      msg.style.display = "block";
      msg.textContent = "登録に失敗しました。時間をおいて再度お試しください。";
      return;
    }

    hideProfileForm();
    await autoToggleCheckin();
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
  if (USE_LIFF) {
    await liff.init({ liffId: LIFF_ID });

    if (!liff.isLoggedIn()) {
      liff.login();
      return;
    }

    const profile = await liff.getProfile();
    currentUserId = profile.userId;
    currentDisplayName = profile.displayName;
  } else {
    currentUserId = LOCAL_DEV_USER.userId;
    currentDisplayName = LOCAL_DEV_USER.displayName;
  }

  const profileReady = await ensureProfile(currentUserId);
  if (!profileReady) {
    return;
  }

  await autoToggleCheckin();
}

async function autoToggleCheckin() {
  try {
    const statusRes = await fetch(`/status?userId=${encodeURIComponent(currentUserId)}`);
    const statusData = await statusRes.json();
    updateCapacityBar(statusData.count, MAX_FALLBACK);

    if (statusData.checkedIn) {
      if (!statusData.canAutoCheckout) {
        const remainSec = Number(statusData.autoCheckoutBlockedSeconds || 0);
        const remainMin = Math.max(1, Math.ceil(remainSec / 60));
        showResultMessage(`チェックイン中です。チェックアウトは${remainMin}分後に可能です。`);
        return;
      }

      const checkoutRes = await fetch("/checkout", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          userId: currentUserId,
          displayName: currentDisplayName,
        }),
      });

      if (!checkoutRes.ok) {
        throw new Error(`checkout failed: ${checkoutRes.status}`);
      }

      const checkoutData = await checkoutRes.json();
      updateCapacityBar(checkoutData.count, MAX_FALLBACK);
      showResultMessage("チェックアウトしました。");
      return;
    }

    if (!statusData.canAutoCheckin) {
      const remainSec = Number(statusData.autoCheckinBlockedSeconds || 0);
      const remainMin = Math.max(1, Math.ceil(remainSec / 60));
      showResultMessage(`チェックアウト後のため、チェックインは${remainMin}分後に可能です。`);
      return;
    }

    const checkinRes = await fetch("/checkin", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        userId: currentUserId,
        displayName: currentDisplayName,
      }),
    });

    if (!checkinRes.ok) {
      throw new Error(`checkin failed: ${checkinRes.status}`);
    }

    const checkinData = await checkinRes.json();
    updateCapacityBar(checkinData.count, MAX_FALLBACK);
    showResultMessage("チェックインが完了しました。");
  } catch (e) {
    console.error(e);
    showResultMessage("チェックイン/チェックアウトに失敗しました。", true);
  }
}

function updateCapacityBar(count, max) {
  const realMax = max;
  const percent = Math.min(100, Math.round((count / realMax) * 100));
  const fill = document.getElementById("capacityFill");
  const text = document.getElementById("capacityText");
  const icon = document.getElementById("capacityIcon");
  const percentLabel = document.getElementById("capacityPercent");

  fill.style.width = `${percent}%`;

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

  percentLabel.textContent = `${percent}%`;
  text.textContent = `混雑度：${count} / ${realMax}（${percent}%）`;
}

const profileSubmitBtn = document.getElementById("profileSubmitBtn");
if (profileSubmitBtn) {
  profileSubmitBtn.addEventListener("click", submitProfile);
}

init();
