const MAX_FALLBACK = 10;

function updateCapacityBar(count, max) {
  const realMax = MAX_FALLBACK;
  const percent = Math.min(100, Math.round((count / realMax) * 100));
  const fill = document.getElementById("capacityFill");
  const text = document.getElementById("capacityText");
  const icon = document.getElementById("capacityIcon");
  const percentLabel = document.getElementById("capacityPercent");

  fill.style.width = percent + "%";

  if (percent > 69) {
    fill.style.background = "linear-gradient(90deg, #f43f5e 0%, #e11d48 100%)"; // 赤
    icon.src = "/hard.png";
  } else if (percent > 49) {
    fill.style.background = "linear-gradient(90deg, #fb923c 0%, #f97316 100%)"; // オレンジ
    icon.src = "/normal.png";
  } else {
    fill.style.background = "linear-gradient(90deg, #22c55e 0%, #16a34a 100%)"; // 緑
    icon.src = "/good.png";
  }

  percentLabel.textContent = percent + "%";
  text.textContent = `混雑度：${count} / ${realMax}（${percent}%）`;
}

async function refresh() {
    const err = document.getElementById("errorMsg");
    const updated = document.getElementById("updatedAt");
    try {
      err.textContent = "";
      const res = await fetch("/count-json", { cache: "no-store" });
      if (!res.ok) throw new Error(`/count-json HTTP ${res.status}`);
      const data = await res.json();
      updateCapacityBar(data.count, data.max || MAX_FALLBACK);
  
      const d = new Date();
      const hh = String(d.getHours()).padStart(2, "0");
      const mm = String(d.getMinutes()).padStart(2, "0");
      const ss = String(d.getSeconds()).padStart(2, "0");
      updated.textContent = `最終更新: ${hh}:${mm}:${ss}`;
    } catch (e) {
      err.textContent = "更新に失敗しました。しばらくしてから再読み込みしてください。";
      console.error(e);
    }
  }

refresh();
setInterval(refresh, 10000);
