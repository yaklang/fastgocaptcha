/**
 * 显示滑动验证码弹窗
 * @param {Object} options - 配置选项
 * @param {string} options.captchaUrl - 获取验证码的URL，默认为'/fastgocaptcha/captcha'
 * @param {string} options.verifyUrl - 验证的URL，默认为'/fastgocaptcha/verify'
 * @param {Function} options.onSuccess - 验证成功的回调函数
 * @param {Function} options.onError - 验证失败的回调函数
 * @param {Function} options.onClose - 弹窗关闭的回调函数
 * @returns {Object} 包含close方法的对象，用于手动关闭弹窗
 */
function showSlideCaptcha(options = {}) {
    // 默认选项
    const defaults = {
        captchaUrl: '/fastgocaptcha/captcha',
        verifyUrl: '/fastgocaptcha/verify',
        onSuccess: () => {},
        onError: () => {},
        onClose: () => {}
    };
    
    // 合并选项
    const settings = {...defaults, ...options};
    
    // 确保依赖的CSS和JS已加载
    function ensureDependenciesLoaded() {
        return new Promise((resolve, reject) => {
            // 检查CSS是否已加载
            if (!document.querySelector('link[href="/static/fastgocaptcha/gocaptcha.global.css"]')) {
                const cssLink = document.createElement('link');
                cssLink.rel = 'stylesheet';
                cssLink.href = '/static/fastgocaptcha/gocaptcha.global.css';
                document.head.appendChild(cssLink);
            }
            
            // 检查JS是否已加载
            if (typeof GoCaptcha === 'undefined') {
                const jsScript = document.createElement('script');
                jsScript.src = '/static/fastgocaptcha/gocaptcha.global.js';
                jsScript.onload = resolve;
                jsScript.onerror = reject;
                document.head.appendChild(jsScript);
            } else {
                resolve();
            }
        });
    }
    
    // 创建模态框
    function createModal() {
        // 创建模态框容器
        const modal = document.createElement('div');
        modal.className = 'slide-captcha-modal';
        modal.style.cssText = `
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background-color: rgba(0, 0, 0, 0.5);
            display: flex;
            justify-content: center;
            align-items: center;
            z-index: 9999;
        `;
        
        // 创建模态框内容
        const modalContent = document.createElement('div');
        modalContent.className = 'slide-captcha-modal-content';
        modalContent.style.cssText = `
            background-color: white;
            padding: 30px;
            border-radius: 10px;
            box-shadow: 0 4px 10px rgba(0, 0, 0, 0.1);
            width: 320px;
            position: relative;
        `;
        
        // 创建关闭按钮
        const closeButton = document.createElement('button');
        closeButton.className = 'slide-captcha-close-btn';
        closeButton.innerHTML = '&times;';
        closeButton.style.cssText = `
            position: absolute;
            top: 10px;
            right: 10px;
            background: none;
            border: none;
            font-size: 24px;
            cursor: pointer;
            color: #999;
        `;
        closeButton.onclick = closeModal;
        
        // 创建标题
        const title = document.createElement('h2');
        title.textContent = '请完成滑动验证';
        title.style.cssText = `
            text-align: center;
            color: #333;
            margin-bottom: 20px;
            font-size: 18px;
        `;
        
        // 创建验证码容器
        const captchaContainer = document.createElement('div');
        captchaContainer.id = 'slide-captcha-container-' + Date.now();
        
        // 添加到DOM
        modalContent.appendChild(closeButton);
        modalContent.appendChild(title);
        modalContent.appendChild(captchaContainer);
        modal.appendChild(modalContent);
        document.body.appendChild(modal);
        
        return {
            modal,
            captchaContainer
        };
    }
    
    // 关闭模态框
    function closeModal() {
        if (modal) {
            document.body.removeChild(modal);
            modal = null;
            settings.onClose();
        }
    }
    
    // 初始化验证码
    function initCaptcha(container) {
        let captchaId = '';
        
        // 创建验证码实例
        const capt = new GoCaptcha.Slide({
            width: 300,
            height: 220,
            text: {
                loading: '加载中...',
                slide: '请拖动滑块完成拼图',
                success: '验证成功',
                error: '验证失败',
                refresh: '刷新验证码'
            }
        });
        
        // 挂载到容器
        capt.mount(container);
        
        // 加载验证码
        function loadCaptcha() {
            fetch(settings.captchaUrl)
                .then(response => {
                    if (!response.ok) {
                        throw new Error('Network response was not ok');
                    }
                    return response.json();
                })
                .then(data => {
                    if (!data.fastgocaptcha_image_base64 || !data.fastgocaptcha_thumb_base64) {
                        throw new Error('Invalid captcha data received');
                    }
                    captchaId = data.fastgocaptcha_id;
                    
                    // 设置验证码数据
                    capt.setData({
                        image: data.fastgocaptcha_image_base64,
                        thumb: data.fastgocaptcha_thumb_base64,
                        thumbWidth: data.fastgocaptcha_thumb_width,
                        thumbHeight: data.fastgocaptcha_thumb_height,
                        thumbX: data.fastgocaptcha_thumb_x,
                        thumbY: data.fastgocaptcha_thumb_y,
                    });
                })
                .catch(err => {
                    console.error('Failed to load captcha:', err);
                    settings.onError('验证码加载失败，请刷新重试');
                });
        }
        
        // 设置事件处理
        capt.setEvents({
            confirm(point, reset) {
                const formData = new FormData();
                formData.append('id', captchaId);
                formData.append('x', point.x);
                
                fetch(settings.verifyUrl, {
                    method: 'POST',
                    body: formData
                })
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        settings.onSuccess(captchaId);
                        setTimeout(closeModal, 1000); // 验证成功后延迟关闭
                    } else {
                        settings.onError('验证失败，请重试');
                        reset();
                        // 重新加载验证码
                        setTimeout(loadCaptcha, 1000);
                    }
                })
                .catch(err => {
                    console.error('Verification failed:', err);
                    settings.onError('验证请求失败，请重试');
                    reset();
                });
            },
            refresh() {
                loadCaptcha();
            }
        });
        
        // 初始加载验证码
        loadCaptcha();
        
        return capt;
    }
    
    // 主流程
    let modal = null;
    let captcha = null;
    
    ensureDependenciesLoaded().then(() => {
        const elements = createModal();
        modal = elements.modal;
        captcha = initCaptcha(elements.captchaContainer);
    }).catch(error => {
        console.error('Failed to load dependencies:', error);
        settings.onError('加载验证组件失败');
    });
    
    // 返回控制对象
    return {
        close: closeModal
    };
}
