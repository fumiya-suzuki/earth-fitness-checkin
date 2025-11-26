document.addEventListener('DOMContentLoaded', function () {
    document.querySelectorAll('.pay-checkbox').forEach(function (cb) {
      cb.addEventListener('change', function () {
        const form = cb.closest('form');
        // hidden の最初の paid=0 はそのまま、チェックされたら paid=1 の値が優先される
        form.submit();
      });
    });
  });
  