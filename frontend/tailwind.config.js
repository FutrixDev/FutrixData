/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ["class"],
  content: ["./index.html", "./src/**/*.{vue,js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        "primary": "#0cdf0f",
        "background-light": "#f5f8f6",
        "background-dark": "#102210",
      },
      boxShadow: {
        'glow': '0 0 20px -5px rgba(12, 223, 15, 0.3)',
        'menu': '0 10px 40px -10px rgba(0,0,0,0.8)',
      }
    },
  },
}
