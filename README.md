# Dokumentasi API Backend Go E-Letter 🚀

Dokumen ini berfungsi sebagai panduan teknis lengkap dan panduan integrasi untuk API backend Go. Dokumen ini menjelaskan arsitektur sistem, organisasi direktori, konfigurasi lingkungan (environment), lapisan basis data (database), alur keamanan, serta menyediakan panduan mendetail tentang bagaimana setiap perangkat lunak klien (Website Next.js, Aplikasi Android, dan Aplikasi Desktop WPF) terhubung dan berinteraksi dengan API.

---

## 🗺️ Ringkasan Arsitektur Sistem

Backend Go dirancang sebagai server REST API yang ringan, berkinerja tinggi, dan bertipe aman (type-safe) menggunakan arsitektur berlapis (layered architecture) yang bersih.

```text
               ┌───────────────────────────────────────────────────┐
               │                   KLIEN                           │
               │  [Next.js Web]   [Kotlin Android]   [WPF Desktop] │
               └───────────────────────────────────────────────────┘
                                         │
                        HTTP / JSON      │ (Bearer JWT / Cookies)
                                         ▼
               ┌───────────────────────────────────────────────────┐
               │               ROUTER GIN GOLANG                   │
               │        (Logger, Recovery, CORS, JWT Auth)         │
               └───────────────────────────────────────────────────┘
                                         │
                                         ▼
               ┌───────────────────────────────────────────────────┐
               │                  HANDLERS                         │
               │    Memproses input, validasi JSON, respon HTTP    │
               └───────────────────────────────────────────────────┘
                                         │
                                         ▼
               ┌───────────────────────────────────────────────────┐
               │                  SERVICES                         │
               │    Logika Bisnis Utama, Alur Kerja, Validasi      │
               └───────────────────────────────────────────────────┘
                                         │
                                         ▼
               ┌───────────────────────────────────────────────────┐
               │                REPOSITORIES                       │
               │    Enkapsulasi Kueri SQL dan Transaksi DB         │
               └───────────────────────────────────────────────────┘
                      │                                     │
                      ▼                                     ▼
        ┌───────────────────────────┐         ┌───────────────────────────┐
        │     BASIS DATA MARIADB    │         │        REDIS CACHE        │
        │    (Penyimpanan Relasional)│         │ (Penyimpanan Rate Limit)  │
        └───────────────────────────┘         └───────────────────────────┘
```

### Teknologi Utama

- **Mesin Utama**: Go 1.22 dengan web framework **Gin Gonic**.
- **Basis Data**: **MariaDB 11.5** (penyimpanan relasional).
- **Caching & Keamanan**: **Redis** (digunakan untuk pembatasan laju masuk/login rate limiting).
- **Notifikasi Real-time**: **Server-Sent Events (SSE)** melalui Event Bus berbasis Go.
- **Autentikasi**: JWT (Access Token + Refresh Token).

---

## 📁 Struktur Direktori

Direktori `backend` mengikuti tata letak proyek Go yang bersih:

```text
backend/
├── cmd/
│   └── api/
│       └── main.go              # Titik masuk aplikasi, injeksi dependensi, memulai server
├── internal/
│   ├── config/                  # Pemuat konfigurasi, inisialisasi DB dan Redis
│   │   ├── config.go            # Definisi struct konfigurasi
│   │   ├── database.go          # Pengaturan connection pooling MariaDB
│   │   └── load.go              # Pemuat file env dengan nilai default dan pembantu parser
│   ├── domain/                  # Entitas inti dan model pemetaan basis data
│   ├── dto/                     # Data Transfer Objects untuk permintaan dan respons
│   ├── handler/                 # HTTP Handlers (controllers) yang mengeksekusi permintaan Web
│   ├── middleware/              # Middleware Logging, CORS, Auth, dan Rate-limiting
│   ├── repository/              # Implementasi kueri SQL dan operasi DB
│   ├── response/                # Utilitas format respons API terstruktur
│   └── service/                 # Implementasi logika bisnis
├── public/                      # Aset file statis
│   └── uploads/                 # Jalur penyimpanan untuk tanda tangan & lampiran
└── routes/
    └── router.go                # Definisi rute API dan grup berbasis peran (role)
```

---

## ⚙️ Konfigurasi Lingkungan (`.env`)

Buat file `.env` di direktori utama `backend/` berdasarkan `.env.example`:

| Kunci (Key)               | Default / Contoh        | Deskripsi                                                           |
| :------------------------ | :---------------------- | :------------------------------------------------------------------ |
| `APP_ENV`                 | `development`           | Mode lingkungan (`development` / `production`).                     |
| `APP_PORT`                | `8080`                  | Port tempat server API Go berjalan.                                 |
| `APP_TIMEZONE`            | `Asia/Jakarta`          | Konteks zona waktu untuk tanggal dan waktu.                         |
| `SCHOOL_CODE`             | `SMKN2SGS`              | Pengidentifikasi sekolah yang digunakan untuk pola penomoran surat. |
| `TRUSTED_PROXIES`         | `127.0.0.1`             | Daftar IP server proxy tepercaya yang dipisahkan koma.              |
| `DB_HOST`                 | `localhost`             | Alamat host server MariaDB.                                         |
| `DB_PORT`                 | `3306`                  | Port server MariaDB.                                                |
| `DB_USER`                 | `root`                  | Username MariaDB.                                                   |
| `DB_PASSWORD`             |                         | Password MariaDB.                                                   |
| `DB_NAME`                 | `db_eletter`            | Nama basis data target.                                             |
| `DB_MAX_OPEN_CONNS`       | `25`                    | Jumlah maksimum koneksi terbuka ke basis data.                      |
| `DB_MAX_IDLE_CONNS`       | `10`                    | Jumlah maksimum koneksi tidak aktif (idle) dalam pool.              |
| `JWT_SECRET`              | `your_jwt_secret`       | Kunci rahasia yang digunakan untuk menandatangani JWT Access Token. |
| `JWT_EXPIRES_IN`          | `30m`                   | Durasi masa berlaku JWT Access Token (misalnya `30m`, `24h`).       |
| `JWT_REFRESH_EXPIRES_IN`  | `720h`                  | Durasi masa berlaku Refresh Token (misalnya `720h` / 30 hari).      |
| `REDIS_HOST`              | `localhost`             | Alamat host server Redis.                                           |
| `REDIS_PORT`              | `6379`                  | Port server Redis.                                                  |
| `REDIS_PASSWORD`          |                         | Password autentikasi Redis.                                         |
| `REDIS_DB`                | `0`                     | Indeks basis data logis Redis.                                      |
| `RATE_LIMIT_MAX_ATTEMPTS` | `5`                     | Upaya login maksimum yang diizinkan sebelum diblokir.               |
| `RATE_LIMIT_WINDOW`       | `5m`                    | Durasi jendela waktu untuk pembatasan laju login.                   |
| `ADMIN_USERNAME`          | `A-001`                 | Username default untuk administrator sistem.                        |
| `ADMIN_PASSWORD`          | `your_password`         | Password untuk akun administrator.                                  |
| `KEPSEK_USERNAME`         | `KS-001`                | Username default untuk Kepala Sekolah.                              |
| `KEPSEK_PASSWORD`         | `your_password`         | Password untuk akun Kepala Sekolah.                                 |
| `CORS_ALLOWED_ORIGINS`    | `http://localhost:3000` | Asal (origins) Cross-Origin yang diizinkan (dipisahkan koma).       |

---

## 🔐 Autentikasi, Keamanan & RBAC

### 1. Sistem Autentikasi Dual-Token JWT

Backend menggunakan dua token untuk alur autentikasi yang aman:

1. **Access Token**: Berumur pendek (misalnya, 30 menit). Dikirim melalui header HTTP `Authorization: Bearer <token>` oleh klien mobile/desktop, atau ditangani di dalam cookie untuk klien web.
2. **Refresh Token**: Berumur panjang (misalnya, 30 hari). Digunakan untuk memperbarui Access Token secara senyap (silently) ketika kedaluwarsa tanpa mengharuskan pengguna login kembali. Pada klien Web, Refresh Token disimpan sebagai cookie secure, `HttpOnly`, `Secure`, dan `SameSite=Lax` untuk melindungi dari serangan XSS.

### 2. Rate Limiting (Pembatasan Laju)

Untuk mencegah serangan brute-force pada endpoint login, server menggunakan pembatas laju yang didukung oleh Redis.

- Rute yang dipantau: `/api/v1/auth/login`, `/api/v1/auth/admin-login`, `/api/v1/auth/kepsek-login`.
- Batas: 5 upaya login dalam jendela waktu bergulir 5 menit per alamat IP.

### 3. Role-Based Access Control (RBAC)

Backend Go memberlakukan pemeriksaan otorisasi endpoint berdasarkan peran pengguna:

- **Admin**: Akses ke semua administrasi sistem, tahun akademik, akun pengguna, dan log audit.
- **Kepsek (Kepala Sekolah)**: Akses ke persetujuan surat akhir, statistik, dan laporan.
- **Teacher (Guru)**: Akses ke layar validasi dan izin untuk mengajukan dispensasi.
- **Student (Siswa)**: Izin untuk meminta surat izin masuk/keluar dan melihat riwayat permintaan mereka sendiri.

#### Sub-Role Guru

Guru juga dapat mendaftarkan sub-peran per tahun akademik:

- **Wali Kelas**: Meninjau permintaan izin untuk kelas mereka.
- **Kapro (Kepala Program Keahlian)**: Meninjau permintaan untuk kompetensi keahlian/jurusan mereka.
- **Tatib (Staf Ketertiban)**: Memberikan persetujuan akhir untuk permintaan izin siswa.

---

## 🔧 Referensi API

Semua permintaan harus diawali dengan `/api/v1`. Rute yang dilindungi memerlukan JWT Access Token valid yang dikirimkan pada header permintaan (`Authorization: Bearer <JWT>`).

### 1. Endpoint Autentikasi Publik

- `POST /register`: Mendaftarkan pengguna baru menggunakan token registrasi verifikasi yang dibuat oleh admin.
- `POST /auth/login`: Login untuk Siswa & Guru menggunakan Email dan Password.
- `POST /auth/admin-login`: Login untuk Administrator (`A-001`).
- `POST /auth/kepsek-login`: Login untuk Kepala Sekolah (`KS-001`).
- `POST /auth/logout`: Mencabut token penyegaran aktif dan mengeluarkan pengguna.
- `POST /auth/refresh`: Memutar (rotate) refresh token dan mengembalikan Access Token baru.
- `POST /auth/forgot-password`: Meminta OTP reset password yang dikirim ke email pengguna.
- `POST /auth/verify-otp`: Memvalidasi OTP verifikasi.
- `POST /auth/reset-password`: Mereset password menggunakan token OTP yang valid.
- `GET /config/school`: Mengambil konfigurasi sekolah yang aktif secara publik.

### 2. Profil Pengguna (Dilindungi)

- `GET /user/profile`: Mengambil detail profil untuk pengguna yang terautentikasi.
- `POST /user/profile`: Alternatif POST untuk mengambil detail profil pengguna.
- `POST /user/update`: Memperbarui bidang profil (misalnya, nama, nomor telepon, tugas kelas).
- `POST /user/signature`: Mengunggah tanda tangan tulisan tangan berbasis file atau base64.
- `POST /user/complete-onboarding`: Menyelesaikan onboarding pendaftaran.

### 3. Permintaan Izin (Dilindungi)

- `GET /permission-requests`: Mencantumkan daftar permintaan izin. Admin/Kepsek mendapatkan data global; Guru mendapatkan permintaan di kelas/jurusan mereka; Siswa hanya mendapatkan milik mereka sendiri.
- `POST /permission-requests`: Mengirimkan permintaan izin baru.
- `PUT /permission-requests`: Memperbarui detail permintaan izin yang aktif.
- `DELETE /permission-requests`: Menghapus permintaan izin (jika belum disetujui).
- `POST /permission-requests/:id/cancel`: Membatalkan permintaan yang diajukan.
- `GET /permission-requests/:id/detail`: Mengambil alur persetujuan lengkap dan detail permintaan izin.
- `POST /approve`: Mengirimkan keputusan persetujuan atau penolakan disertai catatan dan tanda tangan.

### 4. Rute Khusus Surat (Dilindungi)

- `POST /letters/student/create`: Membuat permintaan izin siswa.
- `POST /letters/teacher/create` / `POST /letters/dispensasi`: Membuat permintaan izin guru/dispensasi.
- `GET /letters/student/izin-masuk`: Mengambil surat izin masuk siswa.
- `GET /letters/student/izin-keluar`: Mengambil surat izin keluar siswa.
- `GET /letters/student/dispensasi`: Mengambil status dispensasi siswa.
- `GET /letters/teacher/pending`: Mencantumkan permintaan tertunda yang memerlukan persetujuan guru spesifik ini.
- `GET /letters/kepsek/pending`: Mencantumkan permintaan tertunda yang memerlukan tanda tangan akhir kepala sekolah.
- `GET /letters/kepsek/stats`: Menyediakan statistik global untuk dasbor Kepala Sekolah.

### 5. Server-Sent Events (SSE) (Dilindungi)

- `GET /sse/events`: Koneksi streaming real-time untuk notifikasi push (misalnya, permintaan baru, pembaruan persetujuan).

---

## 🔌 Menghubungkan ke Klien (Panduan Integrasi)

### 1. Panduan Koneksi Klien Web (Aplikasi Next.js)

Website Next.js bertindak sebagai klien web berfitur lengkap. Untuk menghindari masalah eksposur cookie atau CORS, Next.js mengimplementasikan API proxy internal.

#### Mekanisme Autentikasi

1. Pengguna login melalui halaman UI. Permintaan dikirim ke Rute API Next.js `/api/auth/login`.
2. Next.js mem-proxy permintaan ini ke Backend Go `/api/v1/auth/login`.
3. Backend Go mengembalikan `access_token` dan `refresh_token`.
4. Next.js menetapkan `refresh_token` sebagai cookie **HttpOnly** yang aman di peramban (browser) dan mengembalikan `access_token` ke state sisi klien.

#### Mengambil Data dari Endpoint yang Dilindungi (Penanganan Refresh Token)

Klien web menggunakan pembungkus (wrapper) khusus `authenticatedFetch` untuk memeriksa masa berlaku token:

```typescript
// src/lib/authenticated-fetch.ts
export async function authenticatedFetch(
  url: string,
  options: RequestInit = {},
) {
  let token = getAccessToken(); // baca dari memori/state

  // Lampirkan token
  options.headers = {
    ...options.headers,
    Authorization: `Bearer ${token}`,
  };

  let response = await fetch(`${API_BASE_URL}${url}`, options);

  // Jika token kedaluwarsa (401 Unauthorized), lakukan penyegaran senyap (silent refresh)
  if (response.status === 401) {
    const refreshRes = await fetch("/api/auth/refresh", { method: "POST" });
    if (refreshRes.ok) {
      const data = await refreshRes.json();
      setAccessToken(data.accessToken); // Perbarui penyimpanan token

      // Coba kembali permintaan asli
      options.headers["Authorization"] = `Bearer ${data.accessToken}`;
      response = await fetch(`${API_BASE_URL}${url}`, options);
    } else {
      throw new TokenExpiredError("Sesi telah habis, silakan login kembali.");
    }
  }
  return response;
}
```

---

### 2. Panduan Koneksi Klien Android (Kotlin Native)

Aplikasi asli Android menggunakan **Retrofit 2** dan **OkHttp** untuk terhubung langsung ke REST API Go.

#### Konfigurasi Jaringan & Base URL

Karena pengembangan lokal berjalan di `localhost` (yang merujuk pada antarmuka loopback emulator Android itu sendiri, bukan komputer pengembangan Anda), Anda harus mengonfigurasi backend Go untuk mengikat ke alamat IP LAN lokal Anda (misalnya `192.168.1.X`), dan menetapkan IP ini di dalam `RetrofitClient.kt`:

```kotlin
// network/RetrofitClient.kt
object RetrofitClient {
    // Untuk pengembangan lokal pada emulator:
    // private const val BASE_URL = "http://10.0.2.2:8080/api/v1/"

    // Untuk pengembangan lokal pada perangkat fisik:
    private const val BASE_URL = "http://192.168.1.6:8080/api/v1/"

    private val okHttpClient = OkHttpClient.Builder()
        .addInterceptor(HttpLoggingInterceptor().apply {
            level = HttpLoggingInterceptor.Level.BODY
        })
        .connectTimeout(15, TimeUnit.SECONDS)
        .writeTimeout(15, TimeUnit.SECONDS)
        .readTimeout(15, TimeUnit.SECONDS)
        .build()

    val instance: EletterApiService by lazy {
        Retrofit.Builder()
            .baseUrl(BASE_URL)
            .client(okHttpClient)
            .addConverterFactory(GsonConverterFactory.create())
            .build()
            .create(EletterApiService::class.java)
    }
}
```

#### Keamanan Android & Lalu Lintas Cleartext (Cleartext Traffic)

Jika melakukan pengujian secara lokal melalui HTTP (bukan HTTPS), Anda harus secara eksplisit mengizinkan lalu lintas cleartext di manifes Aplikasi Android:

```xml
<!-- app/src/main/AndroidManifest.xml -->
<application
    android:name=".MyApplication"
    android:usesCleartextTraffic="true"
    ... >
    <!-- activities -->
</application>
```

#### Penyimpanan Sesi Android

Aplikasi Android menyimpan data pengguna dan token JWT secara lokal menggunakan `SharedPreferences` dalam mode privat:

```kotlin
// Menyimpan sesi
val sharedPref = context.getSharedPreferences("AppSession", Context.MODE_PRIVATE)
with(sharedPref.edit()) {
    putString("USER_TOKEN", response.token)
    putString("USER_ROLE", response.user?.role)
    putString("USER_NAME", response.user?.full_name)
    apply()
}

// Membaca token untuk header
val token = sharedPref.getString("USER_TOKEN", null)
```

---

### 3. Panduan Koneksi Klien Desktop (Aplikasi C# WPF)

Aplikasi desktop dibangun menggunakan **.NET Framework 4.7.2** dengan WPF dan bergantung pada `System.Net.Http.HttpClient` untuk berkomunikasi dengan API Go.

#### Konfigurasi Base URL

Base URL API dideklarasikan di dalam file konfigurasi `App.config`:

```xml
<!-- App.config -->
<configuration>
  <appSettings>
    <!-- Base URL mengarah ke server API Go -->
    <add key="ApiBaseUrl" value="http://localhost:8080/api/v1" />
  </appSettings>
</configuration>
```

#### Pembungkus HttpClient WPF

Aplikasi desktop menginisialisasi instans `HttpClient` tunggal (singleton) dan mengonfigurasi header otorisasi bearer secara dinamis:

```csharp
// Utilities/Service/ApiClient.cs
using System;
using System.Configuration;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json;

public class ApiClient {
    private static readonly HttpClient _client = new HttpClient();
    private static string _baseUrl = ConfigurationManager.AppSettings["ApiBaseUrl"];

    static ApiClient() {
        _client.DefaultRequestHeaders.Accept.Clear();
        _client.DefaultRequestHeaders.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
    }

    public static void SetBearerToken(string token) {
        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
    }

    public static async Task<ApiResponse<T>> GetAsync<T>(string endpoint) {
        var response = await _client.GetAsync($"{_baseUrl}/{endpoint}");
        var jsonString = await response.Content.ReadAsStringAsync();

        // Tangani kedaluwarsa token
        if (response.StatusCode == System.Net.HttpStatusCode.Unauthorized) {
             // Pemicu logika penyegaran token di sini
        }

        return JsonConvert.DeserializeObject<ApiResponse<T>>(jsonString);
    }
}
```

#### Penyimpanan Token Lokal yang Aman (DPAPI)

Untuk kepatuhan keamanan, klien WPF mengenkripsi token sebelum menyimpannya secara lokal di Windows menggunakan **Data Protection API (DPAPI)**:

```csharp
// Utilities/Helpers/TokenStorage.cs
using System.Security.Cryptography;
using System.Text;

public static class TokenStorage {
    public static void SaveSecureToken(string key, string plainToken) {
        byte[] plainBytes = Encoding.UTF8.GetBytes(plainToken);
        // Enkripsi token menggunakan konteks pengguna saat ini (CurrentUser)
        byte[] encryptedBytes = ProtectedData.Protect(plainBytes, null, DataProtectionScope.CurrentUser);
        string secureBase64 = Convert.ToBase64String(encryptedBytes);

        // Simpan dalam settings / registry
        Properties.Settings.Default[key] = secureBase64;
        Properties.Settings.Default.Save();
    }

    public static string GetSecureToken(string key) {
        string secureBase64 = Properties.Settings.Default[key] as string;
        if (string.IsNullOrEmpty(secureBase64)) return null;

        byte[] encryptedBytes = Convert.FromBase64String(secureBase64);
        byte[] plainBytes = ProtectedData.Unprotect(encryptedBytes, null, DataProtectionScope.CurrentUser);
        return Encoding.UTF8.GetString(plainBytes);
    }
}
```

---

## 🛠️ Menjalankan Backend Go

### Metode 1: Pengembangan Lokal

Pastikan Go 1.22 telah terinstal di sistem Anda.

```bash
# 1. Navigasi ke direktori backend
cd backend

# 2. Sinkronkan dan bersihkan modul Go
go mod tidy

# 3. Salin file konfigurasi .env
cp .env.example .env

# 4. Jalankan aplikasi
go run cmd/api/main.go
```

### Metode 2: Docker Compose (Direkomendasikan)

Anda dapat menyebarkan (deploy) backend bersama dengan basis data MariaDB dan phpMyAdmin secara instan menggunakan Docker.

```bash
# Mulai semua kontainer di latar belakang
docker compose up -d

# Build ulang kontainer backend setelah ada modifikasi kode
docker compose up -d --build backend

# Tinjau log output konsol secara langsung
docker compose logs -f backend
```
