document.addEventListener('DOMContentLoaded', function () {
    // 編集ボタン → フォーム表示＆表示モード非表示
    document.querySelectorAll('.poster-edit-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        const view = btn.closest('.poster-view');
        const cell = view.parentElement;
        const form = cell.querySelector('.poster-edit-form');
  
        view.style.display = 'none';
        form.style.display = 'flex'; // d-flexにしたいのでflex
        const input = form.querySelector('input[name="poster_id"]');
        if (input) {
          input.focus();
          input.select();
        }
      });
    });
  
    // キャンセル → 表示モードに戻す
    document.querySelectorAll('.poster-cancel-btn').forEach(function (btn) {
      btn.addEventListener('click', function () {
        const form = btn.closest('.poster-edit-form');
        const cell = form.parentElement;
        const view = cell.querySelector('.poster-view');
  
        form.style.display = 'none';
        view.style.display = 'flex';
      });
    });
  });
  