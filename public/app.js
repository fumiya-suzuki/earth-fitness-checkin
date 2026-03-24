let currentUserId = null;
let currentDisplayName = "";
const MAX_FALLBACK = 10; // Go側と合わせる
const host = window.location.hostname;
const USE_LIFF = host !== "localhost" && host !== "127.0.0.1" && host !== "::1";
const RESOLVED_LIFF_ID =
  (typeof window !== "undefined" && window.LIFF_ID) ||
  (typeof LIFF_ID !== "undefined" ? LIFF_ID : "");

const LOCAL_DEV_USER = {
  userId: "local-dev-user",
  displayName: "Local Dev",
};

let userGestureed = false;

function setUserGestureed() {
  if (userGestureed) return;
  userGestureed = true;
}

document.addEventListener(
  "click",
  () => {
    setUserGestureed();
  },
  { capture: true, once: true }
);

document.addEventListener(
  "touchstart",
  () => {
    setUserGestureed();
  },
  { capture: true, once: true }
);

function playTapSound() {
  // 音声機能は削除したため、ここは何もしない
}

const resultOverlay = document.createElement("div");
resultOverlay.className = "result-overlay";
const resultEl = document.getElementById("result");
const capacityTextEl = document.getElementById("capacityText");
let resultMessageTimeout = null;
let originalParent = null;
let originalNextSibling = null;

if (resultEl && !document.body.contains(resultOverlay)) {
  // 元の位置を保存
  originalParent = resultEl.parentNode;
  originalNextSibling = resultEl.nextSibling;

  // #result を overlay に移動表示
  resultEl.parentNode?.insertBefore(resultOverlay, resultEl);
  resultOverlay.appendChild(resultEl);

  // overlay クリックで消滅（タイマー任せ）
  resultOverlay.addEventListener("click", () => {
    playTapSound();
    // hideResultMessage(); // 即時消滅を削除
  });

  // resultEl クリックはバブリングを止める（モーダル内部クリックで閉じない）
  resultEl.addEventListener("click", (ev) => {
    ev.stopPropagation();
  });
}

function hideResultMessage() {
  if (!resultEl) return;
  // モーダルを非アクティブに
  if (resultOverlay) {
    resultOverlay.classList.remove("active");
  }
  // resultEl を元の位置に戻す
  if (originalParent && resultEl.parentNode === resultOverlay) {
    originalParent.insertBefore(resultEl, originalNextSibling);
  }
  // スタイルをリセット（ただしメッセージは残す）
  resultEl.style.opacity = "";
  resultEl.style.transform = "translateY(-15px)";
}

function showResultMessage(message, isError = false, actionType = "info") {
  if (!resultEl) {
    return;
  }

  // resultEl をモーダルに移動
  if (resultEl.parentNode !== resultOverlay) {
    resultOverlay.appendChild(resultEl);
  }

  const kind = isError ? "error" : actionType;
  const klass = {
    info: "result-info",
    success: "result-success",
    checkin: "result-checkin",
    checkout: "result-checkout",
    error: "result-error",
  }[kind] || "result-info";

  resultEl.textContent = message;
  resultEl.classList.remove(
    "text-success",
    "text-danger",
    "result-message",
    "result-info",
    "result-checkin",
    "result-checkout",
    "result-success",
    "result-error",
    "active"
  );
  resultEl.classList.add("result-message", klass, "active");
  if (resultOverlay) {
    resultOverlay.classList.add("active");
  }
  resultEl.classList.add("active");

  if (resultMessageTimeout) {
    clearTimeout(resultMessageTimeout);
  }

  resultMessageTimeout = setTimeout(() => {
    hideResultMessage();
  }, 3000);
}

function showInitFailureMessage(message) {
  showResultMessage(message, true);
  if (capacityTextEl) {
    capacityTextEl.textContent = "混雑度を取得できませんでした。再読み込みしてください。";
  }
}

async function reportClientError(event, detail, stage) {
  try {
    await fetch("/client-log", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        event,
        level: "ERROR",
        userId: currentUserId,
        displayName: currentDisplayName,
        path: window.location.pathname,
        userAgent: navigator.userAgent,
        detail,
        stage,
      }),
      keepalive: true,
    });
  } catch (err) {
    console.error("client-log failed", err);
  }
}

// プロフィール取得・表示制御 ------------------------------

async function ensureProfile(userId) {
  currentUserId = userId;

  try {
    const res = await fetch(`/member/profile?userId=${encodeURIComponent(userId)}`);
    if (!res.ok) {
      console.error("profile get error", res.status);
      await reportClientError("profile_fetch_failed", `status=${res.status}`, "ensureProfile");
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
  } catch (e) {
    console.error("profile fetch exception", e);
    await reportClientError("profile_fetch_failed", e.message || String(e), "ensureProfile");
    showProfileForm();
    showInitFailureMessage("会員情報の確認に失敗しました。通信状態をご確認のうえ、再読み込みしてください。");
    return false;
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
    console.error("profile submit error", e);
    await reportClientError("profile_register_failed", e.message || String(e), "submitProfile");
    msg.style.display = "block";
    msg.textContent = "通信エラーが発生しました。";
  }
}

// -------------------------------------------------------
// LIFF 初期化 & チェックイン画面初期化
// -------------------------------------------------------

async function init() {
  try {
    if (USE_LIFF) {
      if (!RESOLVED_LIFF_ID) {
        throw new Error("config_missing");
      }
      if (!window.liff) {
        throw new Error("liff_sdk_missing");
      }

      try {
        await liff.init({ liffId: RESOLVED_LIFF_ID });
      } catch (e) {
        console.error("liff.init failed", e);
        await reportClientError("liff_init_failed", e.message || String(e), "init");
        showInitFailureMessage("LINE初期化に失敗しました。LINEアプリから開き直してください。");
        return;
      }

      if (!liff.isLoggedIn()) {
        console.error("liff not logged in");
        liff.login();
        return;
      }

      try {
        const profile = await liff.getProfile();
        currentUserId = profile.userId;
        currentDisplayName = profile.displayName;
      } catch (e) {
        console.error("liff.getProfile failed", e);
        await reportClientError("liff_profile_failed", e.message || String(e), "init");
        showInitFailureMessage("LINEプロフィール取得に失敗しました。LINEアプリから開き直してください。");
        return;
      }
    } else {
      currentUserId = LOCAL_DEV_USER.userId;
      currentDisplayName = LOCAL_DEV_USER.displayName;
    }

    const profileReady = await ensureProfile(currentUserId);
    if (!profileReady) {
      return;
    }

    await autoToggleCheckin();
  } catch (e) {
    console.error("init failed", e);
    const message = e && e.message ? e.message : String(e);
    await reportClientError("init_failed", message, "init");
    if (message === "config_missing") {
      showInitFailureMessage("設定の読み込みに失敗しました。時間をおいて再読み込みしてください。");
      return;
    }
    if (message === "liff_sdk_missing") {
      showInitFailureMessage("LINE SDKの読み込みに失敗しました。LINEアプリから開き直してください。");
      return;
    }
    showInitFailureMessage("初期化に失敗しました。再読み込みしてください。");
  }
}

async function autoToggleCheckin() {
  try {
    const statusRes = await fetch(`/status?userId=${encodeURIComponent(currentUserId)}`);
    if (!statusRes.ok) {
      console.error("status fetch failed", statusRes.status);
      await reportClientError("status_fetch_failed", `status=${statusRes.status}`, "autoToggleCheckin");
      throw new Error(`status failed: ${statusRes.status}`);
    }
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
        console.error("checkout failed", checkoutRes.status);
        await reportClientError("checkout_failed", `status=${checkoutRes.status}`, "autoToggleCheckin");
        throw new Error(`checkout failed: ${checkoutRes.status}`);
      }

      const checkoutData = await checkoutRes.json();
      updateCapacityBar(checkoutData.count, MAX_FALLBACK);
      showResultMessage("チェックアウトしました。", false, "checkout");
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
      console.error("checkin failed", checkinRes.status);
      await reportClientError("checkin_failed", `status=${checkinRes.status}`, "autoToggleCheckin");
      throw new Error(`checkin failed: ${checkinRes.status}`);
    }

    const checkinData = await checkinRes.json();
    updateCapacityBar(checkinData.count, MAX_FALLBACK);
    showResultMessage("チェックインが完了しました。", false, "checkin");
  } catch (e) {
    console.error("autoToggleCheckin failed", e);
    await reportClientError("auto_toggle_failed", e.message || String(e), "autoToggleCheckin");
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
