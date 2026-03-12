let currentUserId = null;
let currentDisplayName = "";
const MAX_FALLBACK = 10;
const host = window.location.hostname;
const USE_LIFF = host !== "localhost" && host !== "127.0.0.1" && host !== "::1";

const LOCAL_DEV_USER = {
  userId: "local-dev-user",
  displayName: "Local Dev",
};

const resultEl = document.getElementById("result");
const capacityTextEl = document.getElementById("capacityText");
const recoveryPanelEl = document.getElementById("recoveryPanel");
const recoveryTitleEl = document.getElementById("recoveryTitle");
const recoveryDetailEl = document.getElementById("recoveryDetail");
const retryButtonEl = document.getElementById("retryButton");

let initInFlight = false;

function showResultMessage(message, isError = false) {
  if (!resultEl) {
    return;
  }
  resultEl.textContent = message;
  resultEl.classList.remove("text-success", "text-danger");
  resultEl.classList.add(isError ? "text-danger" : "text-success");
}

function setCapacityLoading(message = "混雑度を取得中です...") {
  if (!capacityTextEl) {
    return;
  }
  capacityTextEl.textContent = message;
  capacityTextEl.classList.remove("is-error");
  capacityTextEl.classList.add("is-loading");
}

function setCapacityUnavailable(message = "混雑度を取得できませんでした。再試行してください。") {
  const fill = document.getElementById("capacityFill");
  const percentLabel = document.getElementById("capacityPercent");
  const icon = document.getElementById("capacityIcon");

  if (fill) {
    fill.style.width = "100%";
    fill.style.background = "linear-gradient(90deg, #9ca3af 0%, #6b7280 100%)";
  }
  if (percentLabel) {
    percentLabel.textContent = "--";
  }
  if (icon) {
    icon.src = "./normal.png";
  }
  if (capacityTextEl) {
    capacityTextEl.textContent = message;
    capacityTextEl.classList.remove("is-loading");
    capacityTextEl.classList.add("is-error");
  }
}

function showRecovery(title, detail) {
  if (recoveryPanelEl) {
    recoveryPanelEl.style.display = "block";
  }
  if (recoveryTitleEl) {
    recoveryTitleEl.textContent = title;
  }
  if (recoveryDetailEl) {
    recoveryDetailEl.textContent = detail;
  }
}

function hideRecovery() {
  if (recoveryPanelEl) {
    recoveryPanelEl.style.display = "none";
  }
}

function setRetryButtonState(disabled) {
  if (!retryButtonEl) {
    return;
  }
  retryButtonEl.disabled = disabled;
  retryButtonEl.classList.toggle("btn-loading", disabled);
}

async function reportClientIssue(event, payload = {}, level = "ERROR") {
  try {
    await fetch("/client-log", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        event,
        level,
        userId: currentUserId,
        displayName: currentDisplayName,
        path: window.location.pathname,
        href: window.location.href,
        userAgent: navigator.userAgent,
        ...payload,
      }),
      keepalive: true,
    });
  } catch (error) {
    console.error("client-log failed", error);
  }
}

async function failWithGuidance(code, title, detail, payload = {}) {
  showResultMessage(title, true);
  setCapacityUnavailable();
  showRecovery(title, detail);
  await reportClientIssue(code, payload);
}

function resolveErrorMessage(code) {
  switch (code) {
    case "config_missing":
      return {
        title: "アプリ設定の読み込みに失敗しました。",
        detail: "ページを再読み込みしてください。改善しない場合はLINEから開き直し、しばらくしてから再度お試しください。",
      };
    case "liff_sdk_missing":
      return {
        title: "LINE SDKの読み込みに失敗しました。",
        detail: "通信状態をご確認のうえ、LINEアプリ内ブラウザで開き直してください。SafariやChromeから開いている場合はLINEに戻って開き直してください。",
      };
    case "liff_init_failed":
      return {
        title: "LINE初期化に失敗しました。",
        detail: "LINEアプリを一度閉じてから開き直し、対象URLへ再アクセスしてください。改善しない場合は通信環境を切り替えてください。",
      };
    case "liff_profile_failed":
      return {
        title: "LINEプロフィールを取得できませんでした。",
        detail: "LINEへのログイン状態をご確認のうえ、ページを再読み込みしてください。改善しない場合はLINEアプリから開き直してください。",
      };
    case "profile_fetch_failed":
      return {
        title: "会員情報の確認に失敗しました。",
        detail: "通信状態をご確認のうえ、再試行してください。改善しない場合はスタッフへお声がけください。",
      };
    case "status_fetch_failed":
      return {
        title: "現在の来店状況を取得できませんでした。",
        detail: "人数表示が更新されていません。再試行しても改善しない場合は通信環境を確認し、LINEから開き直してください。",
      };
    case "checkin_failed":
      return {
        title: "チェックイン処理に失敗しました。",
        detail: "再試行しても改善しない場合は、LINEアプリを開き直すかスタッフへお声がけください。",
      };
    case "checkout_failed":
      return {
        title: "チェックアウト処理に失敗しました。",
        detail: "再試行しても改善しない場合は、しばらく時間を置いてから再度お試しください。",
      };
    case "profile_register_failed":
      return {
        title: "プロフィール登録に失敗しました。",
        detail: "入力内容をご確認のうえ再試行してください。改善しない場合は通信環境をご確認ください。",
      };
    default:
      return {
        title: "処理に失敗しました。",
        detail: "再試行しても改善しない場合は、LINEから開き直すかスタッフへお声がけください。",
      };
  }
}

async function handleClientFailure(code, payload = {}) {
  const message = resolveErrorMessage(code);
  await failWithGuidance(code, message.title, message.detail, payload);
}

async function fetchJSON(url, options = {}, logCode = "fetch_failed") {
  let response;
  try {
    response = await fetch(url, options);
  } catch (error) {
    await reportClientIssue(logCode, {
      url,
      detail: error.message,
      phase: "network_error",
    });
    throw error;
  }

  if (!response.ok) {
    await reportClientIssue(logCode, {
      url,
      status: response.status,
      phase: "http_error",
    });
    throw new Error(`${logCode}: ${response.status}`);
  }

  return response.json();
}

async function ensureProfile(userId) {
  currentUserId = userId;

  try {
    const data = await fetchJSON(
      `/member/profile?userId=${encodeURIComponent(userId)}`,
      {},
      "profile_fetch_failed",
    );

    if (!data.exists) {
      showProfileForm();
      return false;
    }

    hideProfileForm();
    return true;
  } catch (error) {
    console.error("profile get error", error);
    showProfileForm();
    await handleClientFailure("profile_fetch_failed", {
      detail: error.message,
      userId,
    });
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
    const data = await fetchJSON(
      "/member/profile",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          userId: currentUserId,
          lastName,
          firstName,
          memberType,
          displayName: currentDisplayName,
        }),
      },
      "profile_register_failed",
    );

    if (!data.ok) {
      throw new Error("profile_register_failed");
    }

    hideProfileForm();
    hideRecovery();
    await autoToggleCheckin();
  } catch (error) {
    console.error(error);
    msg.style.display = "block";
    msg.textContent = "登録に失敗しました。再試行してください。";
    await handleClientFailure("profile_register_failed", {
      detail: error.message,
    });
  }
}

async function init() {
  if (initInFlight) {
    return;
  }

  initInFlight = true;
  setRetryButtonState(true);
  hideRecovery();
  setCapacityLoading();

  try {
    if (USE_LIFF) {
      if (!window.LIFF_ID) {
        throw new Error("config_missing");
      }
      if (!window.liff) {
        throw new Error("liff_sdk_missing");
      }

      try {
        await liff.init({ liffId: LIFF_ID });
      } catch (error) {
        await handleClientFailure("liff_init_failed", { detail: error.message });
        return;
      }

      if (!liff.isLoggedIn()) {
        await reportClientIssue("liff_login_redirect", { detail: "redirect_to_login" }, "INFO");
        liff.login();
        return;
      }

      try {
        const profile = await liff.getProfile();
        currentUserId = profile.userId;
        currentDisplayName = profile.displayName;
      } catch (error) {
        await handleClientFailure("liff_profile_failed", { detail: error.message });
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
  } catch (error) {
    console.error(error);
    const code = error.message || "unexpected_init_error";
    await handleClientFailure(code, { detail: error.stack || error.message });
  } finally {
    initInFlight = false;
    setRetryButtonState(false);
  }
}

async function autoToggleCheckin() {
  try {
    const statusData = await fetchJSON(
      `/status?userId=${encodeURIComponent(currentUserId)}`,
      {},
      "status_fetch_failed",
    );
    updateCapacityBar(statusData.count, MAX_FALLBACK);

    if (statusData.checkedIn) {
      if (!statusData.canAutoCheckout) {
        const remainSec = Number(statusData.autoCheckoutBlockedSeconds || 0);
        const remainMin = Math.max(1, Math.ceil(remainSec / 60));
        showResultMessage(`チェックイン中です。チェックアウトは${remainMin}分後に可能です。`);
        return;
      }

      const checkoutData = await fetchJSON(
        "/checkout",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            userId: currentUserId,
            displayName: currentDisplayName,
          }),
        },
        "checkout_failed",
      );

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

    const checkinData = await fetchJSON(
      "/checkin",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          userId: currentUserId,
          displayName: currentDisplayName,
        }),
      },
      "checkin_failed",
    );

    updateCapacityBar(checkinData.count, MAX_FALLBACK);
    showResultMessage("チェックインが完了しました。");
  } catch (error) {
    console.error(error);
    if (String(error.message || "").startsWith("status_fetch_failed")) {
      await handleClientFailure("status_fetch_failed", { detail: error.message });
      return;
    }
    if (String(error.message || "").startsWith("checkout_failed")) {
      await handleClientFailure("checkout_failed", { detail: error.message });
      return;
    }
    await handleClientFailure("checkin_failed", { detail: error.message });
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
  text.classList.remove("is-loading", "is-error");
}

const profileSubmitBtn = document.getElementById("profileSubmitBtn");
if (profileSubmitBtn) {
  profileSubmitBtn.addEventListener("click", submitProfile);
}

if (retryButtonEl) {
  retryButtonEl.addEventListener("click", () => {
    init();
  });
}

init();
