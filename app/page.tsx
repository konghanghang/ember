import Navbar from './components/home/Navbar'
import HeroSection from './components/home/HeroSection'
import FeaturesSection from './components/home/FeaturesSection'
import QuickStartSection from './components/home/QuickStartSection'
import FAQSection from './components/home/FAQSection'
import Footer from './components/home/Footer'

export default function Home() {
  return (
    <div className="min-h-screen">
      <Navbar />
      <HeroSection />
      <FeaturesSection />
      <QuickStartSection />
      <FAQSection />
      <Footer />
    </div>
  )
}
