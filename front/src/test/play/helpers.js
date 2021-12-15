const randomLengthString = () => Math.random().toString(36).replace(/[^a-z]+/g, '');

const randomId = () => randomLengthString() + randomLengthString()

export {
    randomId
}