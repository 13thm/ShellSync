import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'stores/app_state.dart';
import 'pages/pairing_page.dart';
import 'pages/home_page.dart';

class ShellSyncApp extends StatelessWidget {
  const ShellSyncApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'ShellSync',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF3BA776),
          brightness: Brightness.light,
        ),
        scaffoldBackgroundColor: const Color(0xFFF7F8FA),
        appBarTheme: const AppBarTheme(
          backgroundColor: Colors.white,
          foregroundColor: Color(0xFF1F2329),
          elevation: 0,
          scrolledUnderElevation: 0,
        ),
        filledButtonTheme: FilledButtonThemeData(
          style: FilledButton.styleFrom(
            backgroundColor: const Color(0xFF3BA776),
            foregroundColor: Colors.white,
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
          ),
        ),
      ),
      home: Consumer<AppState>(
        builder: (_, app, __) {
          if (!app.initialized) {
            return const Scaffold(body: Center(child: CircularProgressIndicator()));
          }
          return app.paired ? const HomePage() : const PairingPage();
        },
      ),
    );
  }
}
