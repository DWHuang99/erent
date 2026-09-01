// 静态原型交互脚本：仅处理移动端侧栏开关，后续会迁移为 Vue 组件逻辑
(function () {
    var sidebar = document.querySelector('[data-sidebar]');
    var backdrop = document.querySelector('[data-sidebar-backdrop]');
    var toggleButtons = document.querySelectorAll('[data-sidebar-toggle]');

    if (!sidebar || !backdrop) {
        return;
    }

    function openSidebar() {
        sidebar.classList.add('is-open');
        backdrop.classList.add('is-visible');
    }

    function closeSidebar() {
        sidebar.classList.remove('is-open');
        backdrop.classList.remove('is-visible');
    }

    toggleButtons.forEach(function (btn) {
        btn.addEventListener('click', function () {
            if (sidebar.classList.contains('is-open')) {
                closeSidebar();
            } else {
                openSidebar();
            }
        });
    });

    backdrop.addEventListener('click', closeSidebar);

    window.addEventListener('resize', function () {
        if (window.innerWidth >= 1024) {
            closeSidebar();
        }
    });
})();
