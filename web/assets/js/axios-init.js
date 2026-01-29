axios.defaults.headers.post['Content-Type'] = 'application/x-www-form-urlencoded; charset=UTF-8';
axios.defaults.headers.common['X-Requested-With'] = 'XMLHttpRequest';

axios.interceptors.request.use(
    (config) => {
        if (config.data instanceof FormData) {
            config.headers['Content-Type'] = 'multipart/form-data';
        } else if (config.headers && config.headers['Content-Type'] === 'application/json') {
            // Explicitly set to JSON, don't stringify
            // Data will be automatically serialized by axios
        } else if (typeof config.data === 'object' && config.data !== null) {
            // Check if this is a simple object that should be sent as JSON
            // API endpoints under /panel/api/ will use JSON
            if (config.url && config.url.includes('/panel/api/')) {
                config.headers['Content-Type'] = 'application/json';
                // axios will automatically JSON.stringify the data
            } else {
                // Use form-urlencoded for other endpoints
                config.data = Qs.stringify(config.data, {
                    arrayFormat: 'repeat',
                });
            }
        }
        return config;
    },
    (error) => Promise.reject(error),
);

axios.interceptors.response.use(
    (response) => response,
    (error) => {
        if (error.response) {
            const statusCode = error.response.status;
            // Check the status code
            if (statusCode === 401) { // Unauthorized
                return window.location.reload();
            }
        }
        return Promise.reject(error);
    }
);
