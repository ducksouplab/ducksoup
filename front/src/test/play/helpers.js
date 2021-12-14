const randomId = () => Math.random().toString(36).replace(/[^a-z]+/g, '').substring(0, 8);

export {
    randomId
}