package chat

import (
	"strings"
)

// GetKnowledgeBase returns the knowledge base for the chatbot
func GetKnowledgeBase() []KnowledgeBase {
	return []KnowledgeBase{
		// Donation Process
		{
			ID:       "donation_process",
			Category: "donation",
			Topic:    "process",
			Question: "Bagaimana cara mendonasikan pakaian?",
			Answer:   "Untuk mendonasikan pakaian di SatuLemari: 1) Pastikan pakaian dalam kondisi baik, 2) Foto pakaian dengan jelas, 3) Buat listing dengan kategori yang tepat, 4) Tunggu permintaan dari pengguna, 5) Setujui permintaan dan atur pengambilan.",
			Keywords: []string{"donasi", "cara", "proses", "langkah", "mendonasikan"},
		},
		{
			ID:       "donation_quota",
			Category: "donation",
			Topic:    "quota",
			Question: "Apa itu kuota donasi mingguan?",
			Answer:   "Kuota donasi mingguan adalah batas maksimal 3 item yang dapat diminta oleh pengguna regular setiap minggu. Kuota ini direset setiap hari Senin untuk memastikan distribusi yang adil.",
			Keywords: []string{"kuota", "mingguan", "batas", "maksimal", "reset"},
		},
		{
			ID:       "item_condition",
			Category: "donation",
			Topic:    "condition",
			Question: "Kondisi pakaian seperti apa yang bisa didonasikan?",
			Answer:   "Pakaian yang dapat didonasikan harus dalam kondisi: Sangat Baik (seperti baru), Baik (sedikit bekas pakai), atau Cukup (masih layak pakai). Tidak menerima pakaian yang rusak, kotor, atau tidak layak pakai.",
			Keywords: []string{"kondisi", "excellent", "good", "fair", "sangat baik", "baik", "cukup", "layak", "rusak"},
		},

		// Rental Process
		{
			ID:       "rental_process",
			Category: "rental",
			Topic:    "process",
			Question: "Bagaimana cara menyewa pakaian?",
			Answer:   "Untuk menyewa pakaian: 1) Cari item yang ingin disewa, 2) Periksa ketersediaan dan harga, 3) Buat permintaan sewa dengan tanggal pengambilan dan pengembalian, 4) Tunggu persetujuan mitra, 5) Ambil dan kembalikan sesuai jadwal.",
			Keywords: []string{"sewa", "rental", "cara", "proses", "menyewa"},
		},
		{
			ID:       "rental_duration",
			Category: "rental",
			Topic:    "duration",
			Question: "Berapa lama bisa menyewa pakaian?",
			Answer:   "Durasi sewa fleksibel sesuai kesepakatan dengan partner. Umumnya 1-7 hari untuk acara khusus. Pastikan mengembalikan tepat waktu untuk menghindari denda keterlambatan.",
			Keywords: []string{"durasi", "lama", "sewa", "hari", "pengembalian", "tepat waktu"},
		},

		// Item Categories
		{
			ID:       "categories_list",
			Category: "items",
			Topic:    "categories",
			Question: "Apa saja kategori pakaian yang tersedia?",
			Answer:   "Kategori pakaian di SatuLemari: Pakaian Formal, Pakaian Kasual, Pakaian Olahraga, Pakaian Tradisional, Aksesoris, Pakaian Luar, Alas Kaki, dan Celana.",
			Keywords: []string{"kategori", "jenis", "formal", "kasual", "olahraga", "tradisional", "aksesoris"},
		},

		// Platform Features
		{
			ID:       "search_items",
			Category: "platform",
			Topic:    "search",
			Question: "Bagaimana cara mencari pakaian?",
			Answer:   "Gunakan fitur pencarian dengan filter: kategori, ukuran, warna, kondisi, tipe (donasi/sewa), lokasi, dan rentang harga. Anda juga bisa mencari berdasarkan nama item atau deskripsi.",
			Keywords: []string{"cari", "pencarian", "filter", "ukuran", "warna", "lokasi"},
		},
		{
			ID:       "account_management",
			Category: "platform",
			Topic:    "account",
			Question: "Bagaimana mengelola akun saya?",
			Answer:   "Di halaman profil, Anda dapat: mengubah informasi pribadi, menambah foto profil, mengatur lokasi, melihat riwayat donasi/sewa, dan mengecek kuota mingguan.",
			Keywords: []string{"akun", "profil", "informasi", "foto", "lokasi", "riwayat"},
		},

		// Clothing Care Education
		{
			ID:       "fabric_care_cotton",
			Category: "education",
			Topic:    "fabric_care",
			Question: "Bagaimana cara merawat pakaian katun?",
			Answer:   "Perawatan katun: Cuci dengan air dingin atau hangat, gunakan deterjen lembut, jemur di tempat teduh, setrika dengan suhu sedang. Hindari pemutih yang keras dan pengeringan dengan suhu tinggi.",
			Keywords: []string{"katun", "cotton", "cuci", "air", "deterjen", "setrika", "jemur"},
		},
		{
			ID:       "fabric_care_silk",
			Category: "education",
			Topic:    "fabric_care",
			Question: "Bagaimana cara merawat pakaian sutra?",
			Answer:   "Perawatan sutra: Cuci tangan dengan air dingin dan deterjen khusus sutra, atau dry clean. Jangan diperas, cukup ditekan lembut. Jemur di tempat teduh, setrika dengan suhu rendah.",
			Keywords: []string{"sutra", "silk", "cuci tangan", "air dingin", "dry clean", "deterjen khusus"},
		},
		{
			ID:       "stain_removal",
			Category: "education",
			Topic:    "stain_removal",
			Question: "Bagaimana menghilangkan noda pada pakaian?",
			Answer:   "Tips menghilangkan noda: 1) Segera tangani noda saat masih basah, 2) Gunakan air dingin untuk noda darah/protein, 3) Air hangat untuk noda minyak, 4) Baking soda untuk noda bau, 5) Lemon untuk noda keringat.",
			Keywords: []string{"noda", "stain", "bersihkan", "darah", "minyak", "keringat", "baking soda"},
		},

		// Sustainable Fashion Education
		{
			ID:       "fast_fashion_impact",
			Category: "education",
			Topic:    "sustainability",
			Question: "Apa dampak fast fashion terhadap lingkungan?",
			Answer:   "Fast fashion menyebabkan: polusi air dari pewarna kimia, limbah tekstil yang sulit terurai, konsumsi air berlebihan, emisi karbon tinggi, dan eksploitasi pekerja. SatuLemari membantu mengurangi dampak ini melalui sharing economy.",
			Keywords: []string{"fast fashion", "lingkungan", "polusi", "limbah", "karbon", "sustainability"},
		},
		{
			ID:       "sustainable_practices",
			Category: "education",
			Topic:    "sustainability",
			Question: "Bagaimana berpartisipasi dalam fashion berkelanjutan?",
			Answer:   "Praktik fashion berkelanjutan: 1) Beli pakaian berkualitas yang tahan lama, 2) Rawat pakaian dengan baik, 3) Donasikan atau sewa daripada beli baru, 4) Pilih brand yang etis, 5) Repair daripada buang.",
			Keywords: []string{"berkelanjutan", "sustainable", "kualitas", "rawat", "donasi", "sewa", "repair"},
		},

		// Fashion Tips & Tricks
		{
			ID:       "wardrobe_organization",
			Category: "education",
			Topic:    "tips",
			Question: "Bagaimana mengorganisir lemari pakaian?",
			Answer:   "Tips organisasi lemari: 1) Kelompokkan berdasarkan kategori dan warna, 2) Gunakan hanger yang sama, 3) Lipat dengan teknik Marie Kondo, 4) Rotasi pakaian musiman, 5) Audit rutin untuk donasi.",
			Keywords: []string{"organisasi", "lemari", "kelompok", "hanger", "lipat", "marie kondo", "rotasi"},
		},
		{
			ID:       "outfit_combinations",
			Category: "education",
			Topic:    "tips",
			Question: "Bagaimana membuat kombinasi outfit yang menarik?",
			Answer:   "Tips kombinasi outfit: 1) Ikuti aturan 3 warna maksimal, 2) Mix and match tekstur, 3) Gunakan aksesoris sebagai focal point, 4) Sesuaikan dengan bentuk tubuh, 5) Pertimbangkan occasion dan cuaca.",
			Keywords: []string{"outfit", "kombinasi", "warna", "tekstur", "aksesoris", "bentuk tubuh", "occasion"},
		},
		{
			ID:       "seasonal_dressing",
			Category: "education",
			Topic:    "tips",
			Question: "Bagaimana berpakaian sesuai musim di Indonesia?",
			Answer:   "Tips berpakaian di Indonesia: Musim hujan - pilih bahan quick dry, warna gelap, bawa jaket ringan. Musim kemarau - bahan breathable seperti katun/linen, warna terang, lindungi dari UV.",
			Keywords: []string{"musim", "hujan", "kemarau", "quick dry", "breathable", "katun", "linen", "UV"},
		},

		// Size and Fit
		{
			ID:       "size_guide",
			Category: "items",
			Topic:    "sizing",
			Question: "Bagaimana menentukan ukuran pakaian yang tepat?",
			Answer:   "Panduan ukuran: Ukur lingkar dada, pinggang, dan pinggul. Bandingkan dengan size chart. Untuk atasan, fokus pada lingkar dada. Untuk bawahan, fokus pada pinggang dan pinggul. Jika ragu, pilih ukuran lebih besar.",
			Keywords: []string{"ukuran", "size", "lingkar", "dada", "pinggang", "pinggul", "size chart"},
		},

		// Policies
		{
			ID:       "community_guidelines",
			Category: "platform",
			Topic:    "policies",
			Question: "Apa saja aturan komunitas SatuLemari?",
			Answer:   "Aturan komunitas: 1) Hormati sesama pengguna, 2) Upload foto asli dan jujur, 3) Jaga kebersihan item, 4) Tepat waktu dalam pengambilan/pengembalian, 5) Laporkan masalah ke admin.",
			Keywords: []string{"aturan", "komunitas", "hormati", "foto asli", "kebersihan", "tepat waktu"},
		},
	}
}

// SearchKnowledgeBase searches the knowledge base for relevant entries
func SearchKnowledgeBase(query string, category string) []KnowledgeBase {
	kb := GetKnowledgeBase()
	var results []KnowledgeBase

	queryLower := strings.ToLower(query)

	for _, entry := range kb {
		// Filter by category if specified
		if category != "" && entry.Category != category {
			continue
		}

		// Check if query matches keywords, question, or answer
		match := false

		// Check keywords
		for _, keyword := range entry.Keywords {
			if strings.Contains(strings.ToLower(keyword), queryLower) ||
				strings.Contains(queryLower, strings.ToLower(keyword)) {
				match = true
				break
			}
		}

		// Check question and answer
		if !match {
			if strings.Contains(strings.ToLower(entry.Question), queryLower) ||
				strings.Contains(strings.ToLower(entry.Answer), queryLower) {
				match = true
			}
		}

		if match {
			results = append(results, entry)
		}
	}

	return results
}

// GetChatSuggestions returns predefined chat suggestions
func GetChatSuggestions() ChatSuggestionsResponse {
	return ChatSuggestionsResponse{
		Suggestions: []ChatSuggestion{
			{Text: "Bagaimana cara mendonasikan pakaian?", Category: "donation", Description: "Pelajari proses donasi"},
			{Text: "Bagaimana cara menyewa pakaian?", Category: "rental", Description: "Pelajari proses sewa"},
			{Text: "Apa saja kategori pakaian yang tersedia?", Category: "items", Description: "Lihat kategori item"},
			{Text: "Bagaimana cara merawat pakaian katun?", Category: "education", Description: "Tips perawatan fabric"},
			{Text: "Apa itu kuota donasi mingguan?", Category: "donation", Description: "Pelajari sistem kuota"},
			{Text: "Bagaimana mengorganisir lemari pakaian?", Category: "education", Description: "Tips organisasi"},
			{Text: "Apa dampak fast fashion?", Category: "education", Description: "Edukasi sustainability"},
			{Text: "Bagaimana menentukan ukuran yang tepat?", Category: "items", Description: "Panduan sizing"},
		},
	}
}
